package async_treasury

import (
	"context"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
)

const (
	maxRecordBatchSize = 10
)

var (
	// todo: distributed lock
	treasuryPoolLock sync.Mutex
)

func (p *service) worker(serviceCtx context.Context, state treasury.PoolState, interval time.Duration) error {
	delay := interval
	var cursor query.Cursor

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey{}).(*newrelic.Application)
			m := nr.StartTransaction("async__treasury_pool_service__handle_" + state.String())
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			// Get a batch of records in similar state
			items, err := p.data.GetAllTreasuryPoolsByState(
				tracedCtx,
				state,
				query.WithCursor(cursor),
				query.WithDirection(query.Ascending),
				query.WithLimit(maxRecordBatchSize),
			)
			if err != nil && err != treasury.ErrTreasuryPoolNotFound {
				cursor = query.EmptyCursor
				return err
			}

			// Process the batch of accounts in parallel
			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)
				go func(record *treasury.Record) {
					defer wg.Done()

					err := p.handle(tracedCtx, record)
					if err != nil {
						m.NoticeError(err)
					}
				}(item)
			}
			wg.Wait()

			// Update cursor to point to the next set of pool
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

func (p *service) handle(ctx context.Context, record *treasury.Record) error {
	switch record.State {
	case treasury.PoolStateAvailable:
		return p.handleAvailable(ctx, record)
	default:
		return nil
	}
}

func (p *service) handleAvailable(ctx context.Context, record *treasury.Record) error {
	err := p.updateAccountState(ctx, record)
	if err != nil && err != treasury.ErrStaleTreasuryPoolState {
		return err
	}

	err = p.syncMerkleTree(ctx, record)
	if err != nil {
		return err
	}

	// Runs last, since we have an expectation that our state is up-to-date before
	// saving a new recent root.
	return p.maybeSaveRecentRoot(ctx, record)
}

func (p *service) updateAccountState(ctx context.Context, record *treasury.Record) error {
	if record.DataVersion != splitter_token.DataVersion1 {
		return errors.New("unsupported data version")
	}

	// todo: Use a smarter block. Perhaps from the last finalized payment?
	data, solanaBlock, err := p.data.GetBlockchainAccountDataAfterBlock(ctx, record.Address, record.SolanaBlock)
	if err != nil {
		return errors.Wrap(err, "error querying latest account data from blockchain")
	}

	var unmarshalled splitter_token.PoolAccount
	err = unmarshalled.Unmarshal(data)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling account data")
	}

	err = record.Update(&unmarshalled, solanaBlock)
	if err != nil {
		return err
	}

	return p.data.SaveTreasuryPool(ctx, record)
}
