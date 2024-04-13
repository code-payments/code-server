package async_commitment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
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
	case commitment.StateReadyToOpen:
		return p.handleReadyToOpen(ctx, record)
	case commitment.StateOpen:
		return p.handleOpen(ctx, record)
	case commitment.StateClosed:
		return p.handleClosed(ctx, record)
	default:
		return nil
	}
}

func (p *service) handleReadyToOpen(ctx context.Context, record *commitment.Record) error {
	err := p.maybeMarkTreasuryAsRepaid(ctx, record)
	if err != nil {
		return nil
	}

	shouldOpen, err := p.shouldOpenCommitmentVault(ctx, record)
	if err != nil {
		return err
	}

	if !shouldOpen {
		return p.maybeMarkCommitmentForGC(ctx, record)
	}

	err = p.injectCommitmentVaultManagementFulfillments(ctx, record)
	if err != nil {
		return err
	}

	return markCommitmentAsOpening(ctx, p.data, record.Intent, record.ActionId)
}

func (p *service) handleOpen(ctx context.Context, record *commitment.Record) error {
	err := p.maybeMarkTreasuryAsRepaid(ctx, record)
	if err != nil {
		return nil
	}

	shouldClose, err := p.shouldCloseCommitmentVault(ctx, record)
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
		return nil
	}

	return p.maybeMarkCommitmentForGC(ctx, record)
}

func (p *service) shouldOpenCommitmentVault(ctx context.Context, commitmentRecord *commitment.Record) (bool, error) {
	privacyUpgradeDeadline, err := GetDeadlineToUpgradePrivacy(ctx, p.data, commitmentRecord)
	if err != nil && err != ErrNoPrivacyUpgradeDeadline {
		return false, err
	}

	// The deadline to upgrade privacy has been reached. Open the commitment vault
	// so we can unblock the scheduler from submitting the temporary private transfer.
	if err != ErrNoPrivacyUpgradeDeadline && privacyUpgradeDeadline.Before(time.Now()) {
		return true, nil
	}

	// Otherwise, we must have at least one diverted repayment until we can open the
	// account. This will unblock the scheduler from submitting diverted repayments
	// to this commitment vault.
	count, err := p.data.CountCommitmentRepaymentsDivertedToVault(ctx, commitmentRecord.Vault)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (p *service) shouldCloseCommitmentVault(ctx context.Context, commitmentRecord *commitment.Record) (bool, error) {
	//
	// Part 1: Ensure we either upgraded the temporary private transfer or confirmed it
	//

	// The temporary private transfer could still be scheduled.
	if commitmentRecord.RepaymentDivertedTo == nil {
		fulfillmentRecords, err := p.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, commitmentRecord.Intent, commitmentRecord.ActionId)
		if err != nil {
			return false, err
		}

		if len(fulfillmentRecords) != 1 || *fulfillmentRecords[0].Destination != commitmentRecord.Vault {
			return false, errors.New("fulfillment to check was not found")
		}

		// The temporary private transfer isn't confirmed, so wait for an upgrade
		// or the fulfillment to be played out on the blockchain before closing.
		if fulfillmentRecords[0].State != fulfillment.StateConfirmed {
			return false, nil
		}
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
	if err != nil {
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

	for _, scheduableState := range []fulfillment.State{
		fulfillment.StateUnknown,
		fulfillment.StatePending,
	} {
		numPotentiallyInFlight, err := p.data.GetFulfillmentCountByTypeStateAndAddress(
			ctx,
			fulfillment.PermanentPrivacyTransferWithAuthority,
			scheduableState,
			commitmentRecord.Vault,
		)
		if err != nil {
			return false, err
		}

		// If there are any diverted repayments that are, or potentially will be,
		// scheduled, then leave the vault open.
		if numPotentiallyInFlight > 0 {
			return false, nil
		}
	}

	numConfirmed, err := p.data.GetFulfillmentCountByTypeStateAndAddress(
		ctx,
		fulfillment.PermanentPrivacyTransferWithAuthority,
		fulfillment.StateConfirmed,
		commitmentRecord.Vault,
	)
	if err != nil {
		return false, err
	}

	numFailed, err := p.data.GetFulfillmentCountByTypeStateAndAddress(
		ctx,
		fulfillment.PermanentPrivacyTransferWithAuthority,
		fulfillment.StateFailed,
		commitmentRecord.Vault,
	)
	if err != nil {
		return false, err
	}

	numDiverted, err := p.data.CountCommitmentRepaymentsDivertedToVault(ctx, commitmentRecord.Vault)
	if err != nil {
		return false, err
	}

	// Don't close if there are any failures. A human is needed.
	if numFailed > 0 {
		return false, nil
	}

	// All diverted repayments need to be confirmed before closing the vault
	return numConfirmed >= numDiverted, nil
}

func (p *service) maybeMarkCommitmentForGC(ctx context.Context, commitmentRecord *commitment.Record) error {
	// Can't GC until we know the treasury has been repaid
	if !commitmentRecord.TreasuryRepaid {
		return nil
	}

	// This commitment vault will never be opened, because the funds must have been
	// diverted (or we'd be in the closed state) and a newer commitment vault will
	// be used to divert new repayments.
	if commitmentRecord.State == commitment.StateReadyToOpen {
		return markCommitmentReadyForGC(ctx, p.data, commitmentRecord.Intent, commitmentRecord.ActionId)
	}

	// This commitment vault will never be reopened. We only close the vault when
	// all diverted repayments have been played out. If this commitment has also
	// repaid the treasury, then it must have been through a temporary private transfer
	// flow, or it was diverted itself to a different commitment.
	if commitmentRecord.State == commitment.StateClosed {
		return markCommitmentReadyForGC(ctx, p.data, commitmentRecord.Intent, commitmentRecord.ActionId)
	}

	return nil
}

func (p *service) maybeMarkTreasuryAsRepaid(ctx context.Context, commitmentRecord *commitment.Record) error {
	if commitmentRecord.TreasuryRepaid {
		return nil
	}

	// If we haven't upgraded, then check the status of our commitment vault and whether
	// it indicates the temporary private transfer was repaid.
	if commitmentRecord.RepaymentDivertedTo == nil {
		switch commitmentRecord.State {
		case commitment.StateClosed, commitment.StateReadyToRemoveFromMerkleTree, commitment.StateRemovedFromMerkleTree:
		default:
			// The commitment isn't closed, so we can't say anything about repayment status.
			return nil
		}

		// The commitment is closed, so we know the treasury has been repaid via
		// a temporary private transfer. We know we won't close a commitment until
		// that happens.
		return markTreasuryAsRepaid(ctx, p.data, commitmentRecord.Intent, commitmentRecord.ActionId)
	}

	// Otherwise, check the status of the commitment we've upgraded to and whether
	// the permanent private transfer was repaid.
	divertedRecord, err := p.data.GetCommitmentByVault(ctx, *commitmentRecord.RepaymentDivertedTo)
	if err != nil {
		return err
	}

	switch divertedRecord.State {
	case commitment.StateClosed, commitment.StateReadyToRemoveFromMerkleTree, commitment.StateRemovedFromMerkleTree:
		// The commitment is closed, so we know the treasury has been repiad via a
		// permantent priate transfer. We won't close a commitment until all repayments
		// have been played out.
		return markTreasuryAsRepaid(ctx, p.data, commitmentRecord.Intent, commitmentRecord.ActionId)
	}

	return nil
}

func (p *service) injectCommitmentVaultManagementFulfillments(ctx context.Context, commitmentRecord *commitment.Record) error {
	// Idempotency check to ensure we don't double up on fulfillments
	_, err := p.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.InitializeCommitmentProof, commitmentRecord.Intent, commitmentRecord.ActionId)
	if err == nil {
		return nil
	} else if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return err
	}

	// Commitment vaults have no concept of blocks, intentionally so they're treated
	// equally. This means we need to inject the fulfillments into the same intent
	// where the commitment originated from, even though the private transfer repayment
	// will likely get redirected to another commitment in a different intent. Because
	// we said we'd treat them the same, it's best to think about how we think about
	// repaying with temporary paying, and applying the same heuristic for permanent
	// privacy.
	intentRecord, err := p.data.GetIntent(ctx, commitmentRecord.Intent)
	if err != nil {
		return err
	}

	txnAccounts, txnArgs, err := p.getCommitmentManagementTxnAccountsAndArgs(ctx, commitmentRecord)
	if err != nil {
		return err
	}

	// Construct all fulfillment records
	var fulfillmentsToSave []*fulfillment.Record
	var noncesToReserve []*transaction.SelectedNonce
	for i, txnToMake := range []struct {
		fulfillmentType fulfillment.Type
		ixns            []solana.Instruction
	}{
		{fulfillment.InitializeCommitmentProof, makeInitializeProofInstructions(txnAccounts, txnArgs)},
		{fulfillment.UploadCommitmentProof, makeUploadPartialProofInstructions(txnAccounts, txnArgs, 0, 20)},
		{fulfillment.UploadCommitmentProof, makeUploadPartialProofInstructions(txnAccounts, txnArgs, 21, 41)},
		{fulfillment.UploadCommitmentProof, makeUploadPartialProofInstructions(txnAccounts, txnArgs, 42, 62)}, // todo: Assumes merkle tree of depth 63
		{fulfillment.OpenCommitmentVault, append(
			makeVerifyProofInstructions(txnAccounts, txnArgs),
			makeOpenCommitmentVaultInstructions(txnAccounts, txnArgs)...,
		)},
		{fulfillment.CloseCommitmentVault, makeCloseCommitmentVaultInstructions(txnAccounts, txnArgs)},
	} {
		selectedNonce, err := transaction.SelectAvailableNonce(ctx, p.data, nonce.PurposeInternalServerProcess)
		if err != nil {
			return err
		}

		// note: defer() will only run when the outer function returns, and therefore
		// all of the defer()'s in this loop will be run all at once at the end, rather
		// than at the end of each iteration.
		//
		// Since we are not committing (and therefore consuming) the nonce's until the
		// end of the function, this is desirable. If we released at the end of each
		// iteration, we could potentially acquire the same nonce multiple times for
		// different transactions, which would fail.
		defer func() {
			if err := selectedNonce.ReleaseIfNotReserved(); err != nil {
				p.log.
					WithFields(logrus.Fields{
						"method":        "injectCommitmentVaultManagementFulfillments",
						"nonce_account": selectedNonce.Account.PublicKey().ToBase58(),
						"blockhash":     selectedNonce.Blockhash.ToBase58(),
					}).
					WithError(err).
					Warn("failed to release nonce")
			}

			// This is idempotent regardless of whether the above
			selectedNonce.Unlock()
		}()

		txn, err := transaction.MakeNoncedTransaction(selectedNonce.Account, selectedNonce.Blockhash, txnToMake.ixns...)
		if err != nil {
			return err
		}

		if err := txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes()); err != nil {
			return fmt.Errorf("failed to sign transaction: %w", err)
		}

		intentOrderingIndex := uint64(0)
		fulfillmentOrderingIndex := uint32(i)
		if txnToMake.fulfillmentType == fulfillment.CloseCommitmentVault {
			intentOrderingIndex = uint64(math.MaxInt64)
			fulfillmentOrderingIndex = uint32(0)
		}

		fulfillmentRecord := &fulfillment.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentRecord.IntentType,

			ActionId:   commitmentRecord.ActionId,
			ActionType: action.PrivateTransfer,

			FulfillmentType: txnToMake.fulfillmentType,
			Data:            txn.Marshal(),
			Signature:       pointer.String(base58.Encode(txn.Signature())),

			Nonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
			Blockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),

			Source: commitmentRecord.Vault,

			IntentOrderingIndex:      intentOrderingIndex,
			ActionOrderingIndex:      0,
			FulfillmentOrderingIndex: fulfillmentOrderingIndex,

			DisableActiveScheduling: txnToMake.fulfillmentType != fulfillment.InitializeCommitmentProof,

			State: fulfillment.StateUnknown,
		}

		fulfillmentsToSave = append(fulfillmentsToSave, fulfillmentRecord)
		noncesToReserve = append(noncesToReserve, selectedNonce)
	}

	// Creates all fulfillments and nonce reservations in a single DB transaction
	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		for i := 0; i < len(fulfillmentsToSave); i++ {
			err = noncesToReserve[i].MarkReservedWithSignature(ctx, *fulfillmentsToSave[i].Signature)
			if err != nil {
				return err
			}
		}

		err = p.data.PutAllFulfillments(ctx, fulfillmentsToSave...)
		if err != nil {
			return err
		}

		return nil
	})
}
