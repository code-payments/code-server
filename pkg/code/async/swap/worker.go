package async_swap

import (
	"context"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
)

// todo: Need to handle cancellations, but assume everything is submitted to completion with success
func (p *service) worker(serviceCtx context.Context, state swap.State, interval time.Duration) error {
	var cursor query.Cursor
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__swap_service__handle_" + state.String())
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			items, err := p.data.GetAllSwapsByState(
				tracedCtx,
				state,
				query.WithLimit(p.conf.batchSize.Get(serviceCtx)),
				query.WithCursor(cursor),
			)
			if err != nil {
				cursor = query.EmptyCursor
				return err
			}

			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)

				go func(record *swap.Record) {
					defer wg.Done()

					err := p.handle(tracedCtx, record)
					if err != nil {
						m.NoticeError(err)
					}
				}(item)
			}
			wg.Wait()

			if len(items) > 0 {
				cursor = query.ToCursor(items[len(items)-1].Id)
			} else {
				cursor = query.EmptyCursor
			}

			return nil
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) handle(ctx context.Context, record *swap.Record) error {
	switch record.State {
	case swap.StateFunding:
		return p.handleStateFunding(ctx, record)
	case swap.StateSubmitting:
		return p.handleStateSubmitting(ctx, record)
	}
	return nil
}

func (p *service) handleStateFunding(ctx context.Context, record *swap.Record) error {
	if err := p.validateSwapState(record, swap.StateFunding); err != nil {
		return err
	}

	intentRecord, err := p.data.GetIntent(ctx, record.FundingId)
	if err != nil {
		return errors.Wrap(err, "error getting funding intent record")
	}
	switch intentRecord.State {
	case intent.StateConfirmed:
		return p.markSwapFunded(ctx, record)
	case intent.StateFailed:
		return errors.New("funding intent failed")
	default:
		return nil
	}
}

func (p *service) handleStateSubmitting(ctx context.Context, record *swap.Record) error {
	if err := p.validateSwapState(record, swap.StateSubmitting); err != nil {
		return err
	}

	finalizedTxn, err := p.getTransaction(ctx, record)
	if err != nil && err != solana.ErrSignatureNotFound {
		return errors.Wrap(err, "error getting finalized transaction")
	}

	if finalizedTxn != nil {
		if finalizedTxn.HasErrors || finalizedTxn.ConfirmationState == transaction.ConfirmationFailed {
			return p.markSwapFailed(ctx, record)
		}

		if finalizedTxn.ConfirmationState == transaction.ConfirmationFinalized {
			err = p.updateBalances(ctx, record)
			if err != nil {
				return errors.Wrap(err, "error updating balances")
			}
			return p.markSwapFinalized(ctx, record)
		}
	}

	var txn solana.Transaction
	err = txn.Unmarshal(record.TransactionBlob)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling transaction")
	}

	if base58.Encode(txn.Signature()) != *record.TransactionSignature {
		return errors.New("unexpected transaction signature")
	}

	_, err = p.data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		return errors.Wrap(err, "error submitting transaction")
	}

	return nil
}
