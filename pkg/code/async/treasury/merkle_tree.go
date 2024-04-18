package async_treasury

import (
	"context"
	"sort"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
)

func (p *service) syncMerkleTree(ctx context.Context, treasuryPoolRecord *treasury.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":        "syncMerkleTree",
		"treasury_pool": treasuryPoolRecord.Name,
	})

	// Assumption: There's only one process updating the merkle tree, but there
	// are sufficient safeguards in this method and the store implementations such
	// that we can recover.
	treasuryPoolLock.Lock()
	defer treasuryPoolLock.Unlock()

	//
	// Part 1: Setup the DB-backed merkle tree, creating it if necessary
	//

	log.Trace("setting up db-backed merkle tree")

	merkleTree, err := p.data.LoadExistingMerkleTree(ctx, treasuryPoolRecord.Name, false)
	if err == merkletree.ErrMerkleTreeNotFound {
		treasuryPoolAddressBytes, err := base58.Decode(treasuryPoolRecord.Address)
		if err != nil {
			log.WithError(err).Warn("failure decoding treasury pool address")
			return err
		}

		merkleTree, err = p.data.InitializeNewMerkleTree(
			ctx,
			treasuryPoolRecord.Name,
			treasuryPoolRecord.MerkleTreeLevels,
			[]merkletree.Seed{
				splitter_token.MerkleTreePrefix,
				treasuryPoolAddressBytes,
			},
			false,
		)
		if err != nil {
			log.WithError(err).Warn("failure creating new cached merkle tree")
			return err
		}
	} else if err != nil {
		log.WithError(err).Warn("failure loading cached merkle tree")
		return err
	}

	//
	// Part 2: Check if the merkle tree is already in sync
	//

	log.Trace("checking whether merkle tree is already in sync")

	currentRootNode, err := merkleTree.GetCurrentRootNode(ctx)
	if err != nil && err != merkletree.ErrRootNotFound {
		log.WithError(err).Warn("failure getting current root node")
		return err
	}

	if currentRootNode != nil && currentRootNode.Hash.String() == treasuryPoolRecord.GetMostRecentRoot() {
		log.Trace("merkle tree is already in sync")
		return nil
	}

	//
	// Part 3: Find the fulfillment whose signature acts as the ending checkpoint for
	//         treasury payments. This ensures we create a reasonable and well-defined
	//         upper bound on processing treasury payments.
	//

	log.Trace("looking for ending checkpoint")

	intentRecord, err := p.data.GetLatestSaveRecentRootIntentForTreasury(ctx, treasuryPoolRecord.Address)
	if err == intent.ErrIntentNotFound {
		// Nothing to do, since a recent root has never been saved yet
		log.Trace("waiting for server to save the first recent root")
		return nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting intent record")
		return err
	}

	if intentRecord.State != intent.StateConfirmed {
		log.Trace("last saved recent root intent isn't confirmed")
		return errors.New("last saved recent root intent isn't confirmed")
	}

	// Assumption: There's only one action and fulfillment for a save recent root intent.
	checkpoint, err := p.data.GetAllFulfillmentsByAction(ctx, intentRecord.IntentId, 0)
	if err != nil {
		log.WithError(err).Warn("failure getting fulfillments for action")
		return err
	}

	txnRecord, err := p.getTransaction(ctx, *checkpoint[0].Signature)
	if err != nil {
		log.WithError(err).Warn("failure getting transaction for fulfillment")
		return err
	}

	endingSignature := *checkpoint[0].Signature
	endingBlock := txnRecord.Slot
	log = log.WithField("ending_signature", endingSignature)
	log = log.WithField("ending_block", endingBlock)
	log.Trace("found ending checkpoint")

	//
	// Part 4: Find the fulfillment whose signature acts as the starting checkpoint
	//         for treasury payments. It will act as the exclusive lower bound for
	//         processing treasury payments.
	//

	log.Trace("looking for starting checkpoint")

	// We're going to be operating on Solana blocks to determine order of transaction
	// submission to the treasury pool. The scheduler will schedule transfer with commitment
	// transactions in parallel, so simply observing the order in our intent, fulfillment or
	// payment tables isn't sufficient. We'll need to figure out where we've left off by observing
	// the state of the last leaf added to the merkle tree, which will allow us to infer a
	// signature and Solana block to start from.
	var startingBlock uint64
	var startingSignature string
	leafNode, err := merkleTree.GetLastAddedLeafNode(ctx)
	switch err {
	case nil:
		// Note this may not be a recent root in a treasury's history list. It's very well
		// possible that we failed midway saving leaves to the DB, for example.
		commitmentAddress := base58.Encode(leafNode.LeafValue)
		log := log.WithField("last_leaf_value", commitmentAddress)
		log.Trace("found the last leaf value")

		commitmentRecord, err := p.data.GetCommitmentByAddress(ctx, commitmentAddress)
		if err != nil {
			log.WithError(err).Warn("failure getting commitment record")
			return err
		}

		// Assumption: There's one transfer with commitment transaction per private transfer action
		fulfillmentRecords, err := p.data.GetAllFulfillmentsByAction(ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
		if err != nil {
			log.WithError(err).Warn("failure getting fulfillments from action")
			return err
		}

		var checkpoint *fulfillment.Record
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.State != fulfillment.StateConfirmed {
				continue
			}

			if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
				continue
			}

			checkpoint = fulfillmentRecord
			break
		}
		if checkpoint == nil {
			log.Warn("cannot find fulfillment checkpoint")
			return errors.New("cannot find fulfillment checkpoint")
		}

		paymentRecord, err := p.data.GetPayment(ctx, *checkpoint.Signature, 2)
		if err != nil {
			log.WithError(err).Warn("failure getting payment record from fulfillment")
			return err
		}

		startingSignature = *checkpoint.Signature
		startingBlock = paymentRecord.BlockId
		log = log.WithField("starting_signature", startingSignature)
		log = log.WithField("starting_block", startingBlock)
		log.Trace("found starting checkpoint")
	case merkletree.ErrLeafNotFound:
		// Start from the very beginning (ie. Solana block 0), since the merkle
		// tree is empty.
		log.Trace("starting from block 0 and without a checkpoint because the merkle tree is empty")
	default:
		log.WithError(err).Warn("failure getting latest leaf node")
		return err
	}

	//
	// Part 5: Safely add ordered treasury payments to the merkle tree
	//

	log.Trace("processing ordered treasury payments to add to the merkle tree")

	// Get all payment records from our treasury pool. This assumes the treasury
	// pool vault will only be used as a source for this kind of fulfillment.
	var allTreasuryPayments []*payment.Record
	var cursor query.Cursor
	for {
		endingBlockToQuery := endingBlock + 1
		startingBlockToQuery := startingBlock
		if startingBlockToQuery > 0 {
			startingBlockToQuery--
		}

		paymentRecords, err := p.data.GetPaymentHistoryWithinBlockRange(
			ctx,
			treasuryPoolRecord.Vault,
			startingBlockToQuery,
			endingBlockToQuery,
			query.WithFilter(query.Filter{Value: uint64(payment.TypeSend), Valid: true}),
			query.WithLimit(1000),
			query.WithCursor(cursor),
		)
		if err == payment.ErrNotFound {
			break
		} else if err != nil {
			log.WithError(err).Warn("failure getting payment records")
			return err
		}

		allTreasuryPayments = append(allTreasuryPayments, paymentRecords...)

		cursor = query.ToCursor(paymentRecords[len(paymentRecords)-1].Id)
	}

	semiSortedTreasuryPayments := payment.ByBlock(allTreasuryPayments)
	sort.Sort(semiSortedTreasuryPayments)

	// Group payment records by their block. We cannot determine the order within
	// a given block, yet.
	var sortedBlocks []uint64
	treasuryPaymentsByBlock := make(map[uint64][]*payment.Record)
	for _, treasuryPayment := range semiSortedTreasuryPayments {
		solanaBlock := treasuryPayment.BlockId
		if solanaBlock > endingBlock {
			continue
		}

		_, ok := treasuryPaymentsByBlock[solanaBlock]
		if !ok {
			sortedBlocks = append(sortedBlocks, solanaBlock)
		}

		treasuryPaymentsByBlock[solanaBlock] = append(treasuryPaymentsByBlock[solanaBlock], treasuryPayment)
	}

	// Order all treasury payment records
	var foundStartingCheckpoint, foundEndingCheckpoint bool
	var orderedPaymentsToAdd []*payment.Record
	for _, solanaBlockId := range sortedBlocks {
		log := log.WithField("solana_block", solanaBlockId)
		log.Trace("processing solana block")

		// Break out of the loop if we've hit one form of the defined upper bound
		if foundEndingCheckpoint || solanaBlockId > endingBlock {
			break
		}

		treasuryPaymentsOnBlock := treasuryPaymentsByBlock[solanaBlockId]

		// If there's more than one treasury payment on this block, or it's the
		// block we've saved a recent root on, we need to determine the order using
		// the blockchain block info.
		//
		// todo: We should get this metadata from our DB
		if len(treasuryPaymentsOnBlock) > 1 || solanaBlockId == endingBlock {
			log.Trace("deferring to blockhain for treasury payment ordering")

			orderedSignatures, err := p.data.GetBlockchainBlockSignatures(ctx, solanaBlockId)
			if err != nil {
				log.WithError(err).Warn("failure getting block from blockchain")
				return err
			}

			for _, sig := range orderedSignatures {
				if sig == endingSignature {
					foundEndingCheckpoint = true
					break
				}

				for _, treasuryPayment := range treasuryPaymentsOnBlock {
					if treasuryPayment.TransactionId == sig {
						log.WithField("signature", treasuryPayment.TransactionId).Trace("found treasury payment in block")
						orderedPaymentsToAdd = append(orderedPaymentsToAdd, treasuryPayment)
					}
				}
			}
		} else {
			orderedPaymentsToAdd = append(orderedPaymentsToAdd, treasuryPaymentsOnBlock[0])
		}

		// Anything on or before the starting signature we've checkpointed from has already
		// been added to the merkle tree, so we need to remove them. This can only be done
		// after processing the entire block because we don't know the order of intents
		// within a block.
		if !foundStartingCheckpoint && startingBlock > 0 {
			for i, treasuryPayment := range orderedPaymentsToAdd {
				if treasuryPayment.TransactionId == startingSignature {
					foundStartingCheckpoint = true
					orderedPaymentsToAdd = orderedPaymentsToAdd[i+1:]
					break
				}
			}

			// We still haven't found the starting checkpoint, so clear the entire list
			if !foundStartingCheckpoint {
				orderedPaymentsToAdd = nil
			}
		}
	}

	// Try adding the treasury payments to the merkle tree. There's no guarantee
	// we have all the required information, because our feed of information from
	// the blockchain can be delayed and out-of-order. If this fails, it's best to
	// just wait and try again later after we've observed more of the blockchain's
	// state.
	err = p.safelyAddToMerkleTree(ctx, treasuryPoolRecord, merkleTree, orderedPaymentsToAdd)
	if err == nil {
		recordMerkleTreeSyncedEvent(ctx, treasuryPoolRecord.Name)
	}
	return err
}

// This function expects paymentsToAdd[0] to be the first leaf after the last one saved
// and paymentsToAdd[len(paymentsToAdd)-1] to land on a recent root in the treasury's
// history list. This enables us to optimize the number of times we need to perform a
// simulation down to one.
func (p *service) safelyAddToMerkleTree(ctx context.Context, treasuryPoolRecord *treasury.Record, merkleTree *merkletree.MerkleTree, paymentsToAdd []*payment.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":        "safelyAddToMerkleTree",
		"treasury_pool": treasuryPoolRecord.Name,
	})

	log.Trace("attempting to add leaves to the merkle tree")

	if len(paymentsToAdd) == 0 {
		log.Trace("no treasury payments to add to the merkle tree")
		return errors.New("no treasury payments to add to the merkle tree")
	}

	// For each treasury payment, get the leaf node value we'll add to the merkle tree.
	// In this case, it's the commitment account's address.
	//
	// todo: This can be slow if done serially and can be optimized with parallelization
	var leavesToAdd []merkletree.Leaf
	for _, paymentRecord := range paymentsToAdd {
		log := log.WithField("payment", paymentRecord.TransactionId)
		log.Trace("processing treasury payment")

		commitmentRecord, err := p.getCommitmentFromPayment(ctx, paymentRecord)
		if err != nil {
			log.WithError(err).Warn("failure getting commitment for transaction")
			return err
		}

		commitmentAddressBytes, err := base58.Decode(commitmentRecord.Address)
		if err != nil {
			log.WithError(err).Warn("failure decoding commitment address")
			return err
		}

		log.WithField("leaf_value", commitmentRecord.Address).Trace("calculated leaf value")
		leavesToAdd = append(leavesToAdd, commitmentAddressBytes)
	}

	// Simulate the root hash by applying all leaves in advance to the merkle tree's
	// local state
	simulatedRoot, err := merkleTree.SimulateAddingLeaves(ctx, leavesToAdd)
	if err != nil {
		log.WithError(err).Warn("failure simulating root")
		return err
	}
	log.WithField("simulated_root", simulatedRoot.String()).Trace("simulated root value")

	// Validate the simulated root against the expected observed treasury pool one. We
	// only want to commit values to the DB when we're 100% we have a consistent state
	// with the blockchain.
	actualRecentRoot := simulatedRoot
	expectedRecentRoot := treasuryPoolRecord.GetMostRecentRoot()
	if expectedRecentRoot != actualRecentRoot.String() {
		log.WithFields(logrus.Fields{
			"expected": expectedRecentRoot,
			"actual":   actualRecentRoot.String(),
		}).Info("calculated an incorrect recent root")
		return errors.New("calculated an incorrect recent root")
	}

	log.Trace("merkle tree root simulation was successful")

	// Commit the leaves to the DB now that we've validated we can compute the
	// same expected treasury pool most recent root.
	for _, leaf := range leavesToAdd {
		log := log.WithField("leaf_value", base58.Encode(leaf))
		log.Trace("persisting leaf to db")
		err = merkleTree.AddLeaf(ctx, leaf)
		if err == merkletree.ErrStaleMerkleTree {
			// Should never happen if we're locking correctly, but refresh and try
			// again later.
			log.Warn("merkle tree is unexpectedly stale")
			return err
		} else if err != nil {
			log.WithError(err).Warn("failure persisting leaf to db")
			return err
		}
	}

	return nil
}

// todo: This can likely be optimized with caching or better data modeling/querying
func (p *service) getCommitmentFromPayment(ctx context.Context, paymentRecord *payment.Record) (*commitment.Record, error) {
	fulfillmentRecord, err := p.data.GetFulfillmentBySignature(ctx, paymentRecord.TransactionId)
	if err != nil {
		return nil, err
	}

	return p.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
}

func (p *service) getTransaction(ctx context.Context, signature string) (*transaction.Record, error) {
	record, err := p.data.GetTransaction(ctx, signature)
	if err == transaction.ErrNotFound {
		return p.getTransactionFromBlockchain(ctx, signature)
	}
	return record, err
}

func (p *service) getTransactionFromBlockchain(ctx context.Context, signature string) (*transaction.Record, error) {
	stx, err := p.data.GetBlockchainTransaction(ctx, signature, solana.CommitmentFinalized)
	if err == solana.ErrSignatureNotFound {
		return nil, transaction.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	tx, err := transaction.FromConfirmedTransaction(stx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
