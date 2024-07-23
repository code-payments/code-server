package async_treasury

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
)

// This method is expected to be extremely safe due to the implications of saving
// too many recent roots too fast. There's a high risk of breaking repayments.
// At the very least, we can only save a recent root when the previous one has been
// submitted and we've observed the updated treasury pool state.
//
// todo: We could also wait for proof initializations, but I think it's fine to ignore
// for now. It's highly unlikely we'll progress the treasury so far that the most recent
// root for the proof initialization won't be valid. We can always replace the transactions
// anyways.
func (p *service) maybeSaveRecentRoot(ctx context.Context, treasuryPoolRecord *treasury.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":   "maybeSaveRecentRoot",
		"treasury": treasuryPoolRecord.Name,
	})

	treasuryPoolLock.Lock()
	defer treasuryPoolLock.Unlock()

	//
	// Safety checks part 1:
	// Ensure it's ok to proceed based on the state of the previous intent to save
	// a recent root.
	//

	// todo: This doesn't enable multiple bucketted treasuries being used concurrently
	previousIntentRecord, err := p.data.GetLatestSaveRecentRootIntentForTreasury(ctx, treasuryPoolRecord.Address)
	if err != nil && err != intent.ErrIntentNotFound {
		log.WithError(err).Warn("failure getting previous intent record")
		return err
	}

	// Wait for the previous intent to complete before starting a new one
	if previousIntentRecord != nil && previousIntentRecord.State != intent.StateConfirmed {
		log.Trace("previous intent record isn't confirmed")
		return p.openTreasuryAdvanceFloodGates(ctx, treasuryPoolRecord, previousIntentRecord)
	}

	//
	// Safety checks part 2:
	// Ensure our view of the treasury pool state is up-to-date
	//

	// The general treasury pool account state is not yet up-to-date
	if previousIntentRecord != nil && previousIntentRecord.SaveRecentRootMetadata.PreviousMostRecentRoot == treasuryPoolRecord.GetMostRecentRoot() {
		log.Trace("local treasury state hasn't been synced")
		return nil
	}

	merkleTree, err := p.data.LoadExistingMerkleTree(ctx, treasuryPoolRecord.Name, false)
	if err != nil {
		log.WithError(err).Warn("failure loding merkle tree")
		return err
	}

	currentRootNode, err := merkleTree.GetCurrentRootNode(ctx)
	if err != nil && err != merkletree.ErrRootNotFound {
		return err
	}

	// The merkle tree pool is not yet up-to-date
	if currentRootNode.Hash.String() != treasuryPoolRecord.GetMostRecentRoot() {
		log.Trace("local merkle tree state hasn't been synced")
		return nil
	}

	//
	// Safety checks part 3:
	// Ensure it's ok to proceed based on whether minimum required progress is made
	//

	numAdvancesCollected, err := p.data.GetFulfillmentCountByTypeStateAndAddressAsSource(
		ctx,
		fulfillment.TransferWithCommitment,
		fulfillment.StateUnknown,
		treasuryPoolRecord.Vault,
	)
	if err != nil {
		return err
	}

	// This will catch withdrawal advances that bypass the collection state
	anyNewFinalizedTreasuryAdvances, err := p.anyFinalizedTreasuryAdvancesAfterLastSaveRecentRoot(ctx, treasuryPoolRecord)
	if err != nil {
		log.WithError(err).Warn("failure checking for new finalized treasury advances since last save")
		return err
	}

	// We must collect or have played out at least one advance before we can even
	// think about attempting to save a recent root. Otherwise, we risk saving the
	// same one twice.
	if numAdvancesCollected == 0 && !anyNewFinalizedTreasuryAdvances {
		log.Trace("no treasury advances since last save")
		return nil
	}

	//
	// Safety checks complete. Now we move on to hide in the crowd privacy checks.
	//

	collectionTimeoutStartingPoint := treasuryPoolRecord.LastUpdatedAt // Brand new treasury
	if previousIntentRecord != nil {
		collectionTimeoutStartingPoint = previousIntentRecord.CreatedAt // Existing treasury with at least one recent root saved
	}

	// Did we timeout collecting like-sized transactions? Doing this has a couple benefits:
	//  * Allows temporary privacy cheques to be cashed for unlocked accounts
	//  * Allows server to progress under low load
	// This shouldn't be an issue at high volume anyways, and is a dead-simple heuristic
	// to account for the above points. We can always add something more complex later.
	if time.Since(collectionTimeoutStartingPoint) < p.conf.advanceCollectionTimeout.Get(ctx) {
		minAdvancesRequired := getMinTransactionsForBucketedPrivacyLevel(p.conf.hideInCrowdPrivacyLevel.Get(ctx))

		// Have we collected sufficient like-sized transactions?
		if numAdvancesCollected < minAdvancesRequired {
			log.Tracef("need to collect %d more treasury advances", minAdvancesRequired-numAdvancesCollected)
			return nil
		}
	} else {
		log.Trace("timed out collecting advances")
	}

	//
	// All checks have passed, so we can now move on to begin the process of
	// saving the recent root.
	//

	log.Tracef("initiating process to save recent root with %d advances collected", numAdvancesCollected)

	// Maintaining parity with how clients generate intent IDs
	intentId, err := common.NewRandomAccount()
	if err != nil {
		return err
	}

	intentRecord := &intent.Record{
		IntentId:   intentId.PublicKey().ToBase58(),
		IntentType: intent.SaveRecentRoot,

		InitiatorOwnerAccount: common.GetSubsidizer().PublicKey().ToBase58(),

		SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{
			TreasuryPool:           treasuryPoolRecord.Address,
			PreviousMostRecentRoot: treasuryPoolRecord.GetMostRecentRoot(),
		},

		State: intent.StateUnknown,
	}

	actionRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   0,
		ActionType: action.SaveRecentRoot,

		Source: treasuryPoolRecord.Vault,

		State: action.StatePending,
	}

	selectedNonce, err := transaction.SelectAvailableNonce(ctx, p.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeInternalServerProcess)
	if err != nil {
		log.WithError(err).Warn("failure selecting available nonce")
		return err
	}
	defer selectedNonce.Unlock()

	txn, err := makeSaveRecentRootTransaction(selectedNonce, treasuryPoolRecord)
	if err != nil {
		log.WithError(err).Warn("failure creating transaction")
		return err
	}

	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   actionRecord.ActionId,
		ActionType: actionRecord.ActionType,

		FulfillmentType: fulfillment.SaveRecentRoot,
		Data:            txn.Marshal(),
		Signature:       pointer.String(base58.Encode(txn.Signature())),

		Nonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
		Blockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),

		Source: treasuryPoolRecord.Vault,

		// IntentOrderingIndex unknown until intent record is saved
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		State: fulfillment.StateUnknown,
	}

	// Create the intent in one DB transaction
	err = p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err = p.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure saving intent record")
			return err
		}

		err = p.data.PutAllActions(ctx, actionRecord)
		if err != nil {
			log.WithError(err).Warn("failure saving action record")
			return err
		}

		fulfillmentRecord.IntentOrderingIndex = intentRecord.Id // Unknown until intent record is saved
		err = p.data.PutAllFulfillments(ctx, fulfillmentRecord)
		if err != nil {
			log.Warn("failure saving fulfillment record")
			return err
		}

		err = selectedNonce.MarkReservedWithSignature(ctx, *fulfillmentRecord.Signature)
		if err != nil {
			log.Warn("failure marking nonce reserved with signature")
			return err
		}

		// Intent is pending only after everything's been saved.
		intentRecord.State = intent.StatePending
		err = p.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure marking intent as pending")
		}
		return err
	})
	if err != nil {
		return err
	}

	recordRecentRootIntentCreatedEvent(ctx, treasuryPoolRecord.Name)

	return p.openTreasuryAdvanceFloodGates(ctx, treasuryPoolRecord, intentRecord)
}

func (p *service) openTreasuryAdvanceFloodGates(ctx context.Context, treasuryPoolRecord *treasury.Record, saveRecentRootRecord *intent.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":                "maybeSaveRecentRoot",
		"treasury":              treasuryPoolRecord.Name,
		"intent_ordering_index": saveRecentRootRecord.Id,
	})

	limit := 10 * getMinTransactionsForBucketedPrivacyLevel(p.conf.hideInCrowdPrivacyLevel.Get(ctx))
	for i := 0; i < 10; i++ { // Bounded number of calls so we don't infinitely loop
		count, err := p.data.ActivelyScheduleTreasuryAdvanceFulfillments(
			ctx,
			treasuryPoolRecord.Vault,
			saveRecentRootRecord.Id,
			int(limit),
		)
		if err != nil {
			log.WithError(err).Warn("failure marking treasury advances as actively scheduled")
			return err
		}

		if count == 0 {
			return nil
		}

		log.Tracef("%d treasury advances marked as actively scheduled", count)

		// Don't spam
		time.Sleep(10 * time.Millisecond)
	}

	log.Trace("stopped opening flood gates due to iteration limit")
	return nil
}

func makeSaveRecentRootTransaction(selectedNonce *transaction.SelectedNonce, record *treasury.Record) (solana.Transaction, error) {
	treasuryAddressBytes, err := base58.Decode(record.Address)
	if err != nil {
		return solana.Transaction{}, err
	}

	saveRecentRootInstruction := splitter_token.NewSaveRecentRootInstruction(
		&splitter_token.SaveRecentRootInstructionAccounts{
			Pool:      treasuryAddressBytes,
			Authority: common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:     common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.SaveRecentRootInstructionArgs{
			PoolBump: record.Bump,
		},
	).ToLegacyInstruction()

	// Always use a nonce for this type of transaction. It's way too risky without it,
	// given the implications if we play this out too many times by accident.
	txn, err := transaction.MakeNoncedTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,
		saveRecentRootInstruction,
	)
	if err != nil {
		return solana.Transaction{}, err
	}

	txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())

	return txn, nil
}

func (p *service) anyFinalizedTreasuryAdvancesAfterLastSaveRecentRoot(ctx context.Context, treasuryPoolRecord *treasury.Record) (bool, error) {
	var lowerBoundBlock uint64
	intentRecord, err := p.data.GetLatestSaveRecentRootIntentForTreasury(ctx, treasuryPoolRecord.Address)
	switch err {
	case nil:
		// Still in progress
		if intentRecord.State != intent.StateConfirmed {
			return false, nil
		}

		fulfillmentRecord, err := p.data.GetAllFulfillmentsByAction(ctx, intentRecord.IntentId, 0)
		if err != nil {
			return false, err
		}

		txnRecord, err := p.getTransaction(ctx, *fulfillmentRecord[0].Signature)
		if err != nil {
			return false, err
		}

		lowerBoundBlock = txnRecord.Slot
	case intent.ErrIntentNotFound:
		// New treasury pool without any recent roots saved
		lowerBoundBlock = 0
	default:
		return false, err
	}

	paymentRecords, err := p.data.GetPaymentHistoryWithinBlockRange(
		ctx,
		treasuryPoolRecord.Vault,
		lowerBoundBlock+1,
		math.MaxInt64,
		query.WithFilter(query.Filter{Value: uint64(payment.PaymentType_Send), Valid: true}),
		query.WithLimit(1),
	)
	if err == payment.ErrNotFound || len(paymentRecords) == 0 {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Assumption: To achieve 1 in X privacy with bucketed treasuries we only need
// to observe 2x minimum transactions.
//
// todo: Monitor these assumptions
// todo: We'll likely want to tune this per treasury
func getMinTransactionsForBucketedPrivacyLevel(level uint64) uint64 {
	return 2 * 9 * level
}
