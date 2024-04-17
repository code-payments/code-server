package async_nonce

import (
	"context"
	"sync"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/system"
)

func (p *service) worker(serviceCtx context.Context, state nonce.State, interval time.Duration) error {
	var cursor query.Cursor
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__nonce_service__handle_" + state.String())
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			// Get a batch of nonce records in similar state (e.g. newly created, released, reserved, etc...)
			items, err := p.data.GetAllNonceByState(
				tracedCtx,
				state,
				query.WithLimit(nonceBatchSize),
				query.WithCursor(cursor),
			)
			if err != nil {
				cursor = query.EmptyCursor
				return err
			}

			// Process the batch of nonce accounts in parallel
			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)

				go func(record *nonce.Record) {
					defer wg.Done()

					err := p.handle(tracedCtx, record)
					if err != nil {
						m.NoticeError(err)
					}
				}(item)
			}
			wg.Wait()

			// Update cursor to point to the next set of nonce accounts
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

func (p *service) handle(ctx context.Context, record *nonce.Record) error {

	/*
		Finite state machine:
			States:
				StateUnknown:
					Newly created nonce on our side but the account might not
					exist on Solana.

				StateReleased:
					The account is confirmed to exist but we don't know its
					blockhash yet.

				StateAvailable:
					Available to be used by a payment intent, subscription, or
					other nonce-related transaction.

				StateReserved:
					Reserved by a payment intent, subscription, or other
					nonce-related transaction.

				StateInvalid:
					The nonce account is invalid (e.g. insufficient funds, etc).

			Transitions:
				StateUnknown
					-> StateInvalid
					-> StateReleased
				StateReleased
					-> StateAvailable
				StateAvailable
					-> [externally] StateReserved (nonce used in a new transaction)
				StateReserved
					-> [externally] StateReleased (nonce used in a submitted transaction)
					-> [externally] StateAvailable (nonce will never be submitted in the transaction - eg. it became revoked)
	*/

	// todo: distributed lock on the nonce

	switch record.State {
	case nonce.StateUnknown:
		return p.handleUnknown(ctx, record)
	case nonce.StateReleased:
		return p.handleReleased(ctx, record)
	case nonce.StateAvailable:
		return p.handleAvailable(ctx, record)
	case nonce.StateReserved:
		return p.handleReserved(ctx, record)
	case nonce.StateInvalid:
		return p.handleInvalid(ctx, record)
	default:
		return nil
	}
}

func (p *service) handleUnknown(ctx context.Context, record *nonce.Record) error {
	// Newly created nonces.

	// We're going to the blockchain directly here (super slow btw)
	// because we don't capture the transaction through history yet (it only
	// grabs transfer style transactions for KIN accounts).
	stx, err := p.data.GetBlockchainTransaction(ctx, record.Signature, solana.CommitmentFinalized)
	if err == solana.ErrSignatureNotFound {
		// Check the signature for a potential timeout (e.g. if the nonce account
		// was never created because the blockchain never saw the init/create
		// transaction)
		err := p.checkForMissingTx(ctx, record)
		if err != nil {
			return p.markInvalid(ctx, record)
		}

		return nil
	} else if err != nil {
		return err
	}

	tx, err := transaction.FromConfirmedTransaction(stx)
	if err != nil {
		return err
	}

	if tx.HasErrors || tx.ConfirmationState == transaction.ConfirmationFailed {
		// Somthing went wrong with funding the nonce account. We could try to
		// fix it here but let's mark it invalid to maintain seperation of concern.

		return p.markInvalid(ctx, record)
	}

	if tx.ConfirmationState == transaction.ConfirmationFinalized {
		return p.markReleased(ctx, record)
	}

	// nonce account is not ready yet
	return nil
}

func (p *service) handleReleased(ctx context.Context, record *nonce.Record) error {
	// Nonces that exist but we don't yet know their stored blockhash.

	txn, err := p.getTransaction(ctx, record.Signature)
	if err != nil {
		return err
	}

	// Sanity check the transaction is in a finalized or failed state
	if txn.ConfirmationState != transaction.ConfirmationFinalized && txn.ConfirmationState != transaction.ConfirmationFailed {
		return nil
	}

	// Always get the account's state after the transaction's block to avoid
	// having RPC nodes that are behind provide stale finalized data.
	rawData, _, err := p.data.GetBlockchainAccountDataAfterBlock(ctx, record.Address, txn.Slot)
	if err != nil {
		return err
	}

	if len(rawData) != system.NonceAccountSize {
		// RPC call failed or something (maybe this node has no history?)
		return ErrInvalidNonceAccountSize
	}

	var data system.NonceAccount
	err = data.Unmarshal(rawData)
	if err != nil {
		return err
	}

	nextBlockhash := base58.Encode(data.Blockhash)

	// Precautionary safety check, since it's an easy validation
	if record.Blockhash == nextBlockhash {
		return nil
	}

	// Clear the old signature, we don't need it anymore
	record.Signature = ""
	record.Blockhash = nextBlockhash

	return p.markAvailable(ctx, record)
}

func (p *service) handleReserved(ctx context.Context, record *nonce.Record) error {
	// Nothing to do here
	return nil
}

func (p *service) handleAvailable(ctx context.Context, record *nonce.Record) error {
	// Nothing to do here
	return nil
}

func (p *service) handleInvalid(ctx context.Context, record *nonce.Record) error {
	// We could try to recover this nonce account here but let's just leave it
	// as is for further investigation.
	return nil
}
