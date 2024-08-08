package async_sequencer

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/retry"
)

func (p *service) worker(serviceCtx context.Context, state fulfillment.State, interval time.Duration) error {
	var cursor query.Cursor
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__sequencer_service__handle_" + state.String())
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			// todo: proper config to tune states individually
			var limit uint64
			switch state {
			case fulfillment.StatePending:
				limit = 100 // todo: we'll likely want to up this one, but also rate limit our send/getSignature RPC calls
			default:
				limit = 100
			}

			// Get a batch of records in similar state (e.g. newly created, released, reserved, etc...)
			items, err := p.data.GetAllFulfillmentsByState(
				tracedCtx,
				state,
				false, // Don't poll for fulfillments that have active scheduling disabled
				query.WithLimit(limit),
				query.WithCursor(cursor),
			)
			if err != nil {
				cursor = query.EmptyCursor
				return err
			}

			// Process the batch of fulfillments in parallel
			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)

				go func(record *fulfillment.Record) {
					defer wg.Done()

					err := p.handle(tracedCtx, record)
					if err != nil {
						m.NoticeError(err)
					}
				}(item)
			}
			wg.Wait()

			// Update cursor to point to the next set of fulfillments
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

func (p *service) handle(ctx context.Context, record *fulfillment.Record) error {
	err := p.checkPreconditions(ctx, record)
	if err != nil {
		// Preconditions failed or we could not get the lock, go do something else
		return err
	}

	switch record.State {
	case fulfillment.StateUnknown:
		return p.handleUnknown(ctx, record)
	case fulfillment.StatePending:
		return p.handlePending(ctx, record)
	case fulfillment.StateConfirmed:
		return p.handleConfirmed(ctx, record)
	case fulfillment.StateFailed:
		return p.handleFailed(ctx, record)
	case fulfillment.StateRevoked:
		return p.handleRevoked(ctx, record)
	default:
		return nil
	}
}

func (p *service) checkPreconditions(ctx context.Context, record *fulfillment.Record) error {
	if false {
		// todo: get a distributed lock on the intent from Redis
		return ErrCouldNotGetIntentLock
	}

	return nil
}

func (p *service) handleUnknown(ctx context.Context, record *fulfillment.Record) error {
	handler, ok := p.fulfillmentHandlersByType[record.FulfillmentType]
	if !ok {
		return errors.Errorf("no fulfillment handler for %d type", record.FulfillmentType)
	}

	// Sanity check the fulfillment record. There should be data for a signed
	// transaction when it's not made on demand.
	if !handler.SupportsOnDemandTransactions() && (record.Signature == nil || len(*record.Signature) == 0) {
		return errors.New("fulfillment doesn't support on demand transaction creation")
	}

	isRevoked, nonceUsed, err := handler.IsRevoked(ctx, record)
	if err != nil {
		return err
	} else if isRevoked {
		return p.markFulfillmentRevoked(ctx, record, nonceUsed)
	}

	// Check if this fulfillment is scheduled for submission to the blockchain
	isScheduled, err := p.scheduler.CanSubmitToBlockchain(ctx, record)
	if err != nil {
		return err
	} else if !isScheduled {
		return nil
	}

	return p.markFulfillmentPending(ctx, record)
}

func (p *service) handlePending(ctx context.Context, record *fulfillment.Record) error {
	fulfillmentHandler, ok := p.fulfillmentHandlersByType[record.FulfillmentType]
	if !ok {
		return errors.Errorf("no fulfillment handler for %d type", record.FulfillmentType)
	}

	actionHandler, ok := p.actionHandlersByType[record.ActionType]
	if !ok {
		return errors.Errorf("no action handler for %d type", record.ActionType)
	}

	intentHandler, ok := p.intentHandlersByType[record.IntentType]
	if !ok {
		return errors.Errorf("no intent handler for %d type", record.IntentType)
	}

	// Check on the status of the transaction
	tx, err := p.getTransaction(ctx, record)
	if err != nil && err != transaction.ErrNotFound {
		return err
	}

	if tx != nil {
		if tx.HasErrors || tx.ConfirmationState == transaction.ConfirmationFailed {
			recovered, err := fulfillmentHandler.OnFailure(ctx, record, tx)
			if err != nil {
				return err
			}

			if recovered {
				return nil
			}

			err = actionHandler.OnFulfillmentStateChange(ctx, record, fulfillment.StateFailed)
			if err != nil {
				return err
			}

			err = intentHandler.OnActionUpdated(ctx, record.Intent)
			if err != nil {
				return err
			}

			// By design is the last thing so we can retry all logic
			return p.markFulfillmentFailed(ctx, record)
		}

		if tx.ConfirmationState == transaction.ConfirmationFinalized {
			err := actionHandler.OnFulfillmentStateChange(ctx, record, fulfillment.StateConfirmed)
			if err != nil {
				return err
			}

			err = intentHandler.OnActionUpdated(ctx, record.Intent)
			if err != nil {
				return err
			}

			err = fulfillmentHandler.OnSuccess(ctx, record, tx)
			if err != nil {
				return err
			}

			// By design is the last thing so we can retry all logic
			return p.markFulfillmentConfirmed(ctx, record)
		}
	}

	// We're still pending

	// Create the transaction on demand if it's supported
	if record.Signature == nil {
		if !fulfillmentHandler.SupportsOnDemandTransactions() {
			return errors.New("unexpected scheduled fulfillment without transaction data")
		}

		selectedNonce, err := transaction_util.SelectAvailableNonce(ctx, p.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeOnDemandTransaction)
		if err != nil {
			return err
		}
		defer func() {
			selectedNonce.ReleaseIfNotReserved()
			selectedNonce.Unlock()
		}()

		err = p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
			txn, err := fulfillmentHandler.MakeOnDemandTransaction(ctx, record, selectedNonce)
			if err != nil {
				return err
			}

			record.Signature = pointer.String(base58.Encode(txn.Signature()))
			record.Nonce = pointer.String(selectedNonce.Account.PublicKey().ToBase58())
			record.Blockhash = pointer.String(base58.Encode(selectedNonce.Blockhash[:]))
			record.Data = txn.Marshal()

			err = selectedNonce.MarkReservedWithSignature(ctx, *record.Signature)
			if err != nil {
				return err
			}

			err = p.data.UpdateFulfillment(ctx, record)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	// Re-broadcast the transaction (could be the first time)
	if !p.conf.disableTransactionSubmission.Get(ctx) {
		return p.sendToBlockchain(ctx, record)
	}

	return nil
}

func (p *service) handleConfirmed(ctx context.Context, record *fulfillment.Record) error {
	// Nothing to do here. We're done...
	return nil
}

func (p *service) handleFailed(ctx context.Context, record *fulfillment.Record) error {
	// Nothing to do here. We're done...
	return nil
}

func (p *service) handleRevoked(ctx context.Context, record *fulfillment.Record) error {
	// Nothing to do here. We're done...
	return nil
}
