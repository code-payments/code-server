package async_commitment

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
)

//
// Disclaimer:
// State transition logic is tightly coupled to assumptions of logic for how we
// move between states and how we pick a commitment account to divert repayments
// to. This simplifies the local logic, but does mean we need to be careful making
// updates.
//

const (
	maxRecordBatchSize = 100
)

var (
	// Timeout when we know SubmitIntent won't select a commitment as a candidate
	// for a privacy upgrade. Don't lower this any further.
	//
	// todo: configurable
	privacyUpgradeCandidateSelectionTimeout = 10 * time.Minute
)

var (
	ErrNoPrivacyUpgradeDeadline = errors.New("no privacy upgrade deadline for commitment")
)

func (p *service) worker(serviceCtx context.Context, state commitment.State, interval time.Duration) error {
	delay := interval
	var cursor query.Cursor

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__commitment_service__handle_" + state.String())
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			// Get a batch of records in similar state
			items, err := p.data.GetAllCommitmentsByState(
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

			// Process the batch of commitments in parallel
			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)
				go func(record *commitment.Record) {
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

// todo: needs to lock a distributed lock
func (p *service) handle(ctx context.Context, record *commitment.Record) error {
	switch record.State {
	case commitment.StateOpen:
		return p.handleOpen(ctx, record)
	case commitment.StateClosed:
		return p.handleClosed(ctx, record)
	default:
		return nil
	}
}

func (p *service) handleOpen(ctx context.Context, record *commitment.Record) error {
	err := p.maybeMarkTreasuryAsRepaid(ctx, record)
	if err != nil {
		return err
	}

	shouldClose, err := p.shouldCloseCommitment(ctx, record)
	if err != nil {
		return err
	}

	if shouldClose {
		return markCommitmentAsClosing(ctx, p.data, record.Intent, record.ActionId)
	}

	return nil
}

func (p *service) handleClosed(ctx context.Context, record *commitment.Record) error {
	err := p.maybeMarkTreasuryAsRepaid(ctx, record)
	if err != nil {
		return err
	}

	return p.maybeMarkCommitmentForGC(ctx, record)
}

func (p *service) shouldCloseCommitment(ctx context.Context, commitmentRecord *commitment.Record) (bool, error) {
	if commitmentRecord.State != commitment.StateOpen {
		return false, nil
	}

	//
	// Part 1: Ensure we either upgraded the temporary private transfer or confirmed it
	//

	if commitmentRecord.RepaymentDivertedTo == nil && !commitmentRecord.TreasuryRepaid {
		return false, nil
	}

	//
	// Part 2: Ensure we won't select this commitment vault for any new upgrades
	//
	// Note: As a result, this is directly tied to what we do in SubmitIntent to
	// select a commitment account to divert funds to.
	//

	merkleTree, err := getCachedMerkleTreeForTreasury(ctx, p.data, commitmentRecord.Pool)
	if err != nil {
		return false, err
	}

	commitmentAddressBytes, err := base58.Decode(commitmentRecord.Address)
	if err != nil {
		return false, err
	}

	leafNode, err := merkleTree.GetLeafNode(ctx, commitmentAddressBytes)
	if err == merkletree.ErrLeafNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}

	nexLeafNode, err := merkleTree.GetLeafNodeByIndex(ctx, leafNode.Index+1)
	if err == merkletree.ErrLeafNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if time.Since(nexLeafNode.CreatedAt) < privacyUpgradeCandidateSelectionTimeout {
		// There's a newer commitment, so the current one isn't a candidate to divert
		// to anymore. Wait until it reaches a certain age to avoid any chance of a
		// race condition with using cached merkle trees in SubmitIntent. This time check
		// is exactly why we query for the next leaf versus say the latest.
		//
		// todo: Do something smarter when we have distributed locks.
		return false, nil
	}

	//
	// Part 3: Ensure all upgraded private transfers going to this commitment have been confirmed
	//

	numPendingDivertedRepayments, err := p.data.CountPendingCommitmentRepaymentsDivertedToCommitment(ctx, commitmentRecord.Address)
	if err != nil {
		return false, err
	}

	// All diverted repayments need to be confirmed before closing the commitment
	if numPendingDivertedRepayments > 0 {
		return false, nil
	}

	// todo: There isn't a way to close commitments yet in the VM
	return false, nil
}

func (p *service) maybeMarkCommitmentForGC(ctx context.Context, commitmentRecord *commitment.Record) error {
	// Can't GC until we know the treasury has been repaid
	if !commitmentRecord.TreasuryRepaid {
		return nil
	}

	// The commitment is closed because it has reached a terminal state and we
	// are done with it. All temporary and permaenent cheques that are targets
	// for this commitment should be cashed. Proceed to GC.
	if commitmentRecord.State == commitment.StateClosed {
		return markCommitmentReadyForGC(ctx, p.data, commitmentRecord.Intent, commitmentRecord.ActionId)
	}

	return nil
}

func (p *service) maybeMarkTreasuryAsRepaid(ctx context.Context, commitmentRecord *commitment.Record) error {
	if commitmentRecord.TreasuryRepaid {
		return nil
	}

	fulfillmentTypeToCheck := fulfillment.TemporaryPrivacyTransferWithAuthority
	if commitmentRecord.RepaymentDivertedTo != nil {
		fulfillmentTypeToCheck = fulfillment.PermanentPrivacyTransferWithAuthority
	}

	fulfillmentRecords, err := p.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillmentTypeToCheck, commitmentRecord.Intent, commitmentRecord.ActionId)
	if err != nil {
		return err
	}

	// The cheque hasn't been cashed, so we cannot mark the treasury as being repaid
	if fulfillmentRecords[0].State != fulfillment.StateConfirmed {
		return nil
	}

	return nil
}
