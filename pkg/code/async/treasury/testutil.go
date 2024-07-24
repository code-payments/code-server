package async_treasury

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/solana/system"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx          context.Context
	data         code_data.Provider
	treasuryPool *treasury.Record
	merkleTree   *merkletree.MerkleTree
	worker       *service
	subsidizer   *common.Account
	nextBlock    uint64
}

func setup(t *testing.T, testOverrides *testOverrides) *testEnv {
	ctx := context.Background()

	db := code_data.NewTestDataProvider()

	subsidizer := testutil.SetupRandomSubsidizer(t, db)

	treasuryPoolAddress := testutil.NewRandomAccount(t)
	treasuryPool := &treasury.Record{
		DataVersion: splitter_token.DataVersion1,

		Name: "test-pool",

		Address: treasuryPoolAddress.PublicKey().ToBase58(),
		Bump:    123,

		Vault:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		VaultBump: 100,

		Authority: subsidizer.PublicKey().ToBase58(),

		MerkleTreeLevels: 63,

		CurrentIndex:    1,
		HistoryListSize: 5,

		SolanaBlock: 1,

		State: treasury.TreasuryPoolStateAvailable,
	}

	merkleTree, err := db.InitializeNewMerkleTree(
		ctx,
		treasuryPool.Name,
		treasuryPool.MerkleTreeLevels,
		[]merkletree.Seed{
			splitter_token.MerkleTreePrefix,
			treasuryPoolAddress.PublicKey().ToBytes(),
		},
		false,
	)
	require.NoError(t, err)

	rootNode, err := merkleTree.GetCurrentRootNode(ctx)
	require.NoError(t, err)
	for i := 0; i < int(treasuryPool.HistoryListSize); i++ {
		treasuryPool.HistoryList = append(treasuryPool.HistoryList, hex.EncodeToString(rootNode.Hash))
	}
	require.NoError(t, db.SaveTreasuryPool(ctx, treasuryPool))

	return &testEnv{
		ctx:          ctx,
		data:         db,
		treasuryPool: treasuryPool,
		merkleTree:   merkleTree,
		worker:       New(db, withManualTestOverrides(testOverrides)).(*service),
		subsidizer:   subsidizer,
		nextBlock:    1,
	}
}

func (e *testEnv) simulateMostRecentRoot(t *testing.T, intentState intent.State, commitmentRecords []*commitment.Record) {
	intentRecord := &intent.Record{
		IntentId:              testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType:            intent.SaveRecentRoot,
		InitiatorOwnerAccount: e.subsidizer.PublicKey().ToBase58(),
		SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{
			TreasuryPool:           e.treasuryPool.Address,
			PreviousMostRecentRoot: e.treasuryPool.GetMostRecentRoot(),
		},
		State: intentState,
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intent.SaveRecentRoot,

		ActionId:   0,
		ActionType: action.SaveRecentRoot,

		FulfillmentType: fulfillment.SaveRecentRoot,
		Data:            []byte("data"),
		Signature:       pointer.String(fmt.Sprintf("sig%d", rand.Uint64())),

		Nonce:     pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		Blockhash: pointer.String("bh"),

		Source: e.treasuryPool.Vault,

		State: fulfillment.StateUnknown,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecord))

	txnRecord := &transaction.Record{
		Signature:         *fulfillmentRecord.Signature,
		Slot:              e.getNextBlock(),
		ConfirmationState: transaction.ConfirmationFinalized,
	}
	require.NoError(t, e.data.SaveTransaction(e.ctx, txnRecord))

	e.simulateTreasuryPoolUpdated(t, commitmentRecords)
}

func (e *testEnv) simulateTreasuryPoolUpdated(t *testing.T, commitmentRecords []*commitment.Record) {
	var leavesToSimulate []merkletree.Leaf
	for _, commitmentRecord := range commitmentRecords {
		addressBytes, err := base58.Decode(commitmentRecord.Address)
		require.NoError(t, err)

		leavesToSimulate = append(leavesToSimulate, addressBytes)
	}
	hash, err := e.merkleTree.SimulateAddingLeaves(e.ctx, leavesToSimulate)
	require.NoError(t, err)

	e.treasuryPool.CurrentIndex = (e.treasuryPool.CurrentIndex + 1) % e.treasuryPool.HistoryListSize
	e.treasuryPool.HistoryList[e.treasuryPool.CurrentIndex] = hex.EncodeToString(hash)
	e.treasuryPool.SolanaBlock = e.getNextBlock()
	require.NoError(t, e.data.SaveTreasuryPool(e.ctx, e.treasuryPool))
}

func (e *testEnv) simulateAddingLeaves(t *testing.T, commitmentRecords []*commitment.Record) {
	for _, commitmentRecord := range commitmentRecords {
		addressBytes, err := base58.Decode(commitmentRecord.Address)
		require.NoError(t, err)

		require.NoError(t, e.merkleTree.AddLeaf(e.ctx, addressBytes))
	}
}

func (e *testEnv) simulateCommitments(t *testing.T, count int, recentRoot string, state commitment.State) []*commitment.Record {
	var commitmentRecords []*commitment.Record
	for i := 0; i < count; i++ {
		commitmentRecord := &commitment.Record{
			Address: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

			Pool:       e.treasuryPool.Address,
			RecentRoot: recentRoot,

			Transcript:  "transcript",
			Destination: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Amount:      kin.ToQuarks(1),

			Intent:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			ActionId: rand.Uint32(),

			Owner: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

			State: state,
		}
		require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))
		commitmentRecords = append(commitmentRecords, commitmentRecord)

		fulfillmentRecord := &fulfillment.Record{
			Intent:     commitmentRecord.Intent,
			IntentType: intent.SendPrivatePayment,

			ActionId:   commitmentRecord.ActionId,
			ActionType: action.PrivateTransfer,

			FulfillmentType: fulfillment.TransferWithCommitment,
			Data:            []byte("data"),
			Signature:       pointer.String(fmt.Sprintf("sig%d", rand.Uint64())),

			Nonce:     pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
			Blockhash: pointer.String("bh"),

			Source:      e.treasuryPool.Vault,
			Destination: &commitmentRecord.Destination,

			DisableActiveScheduling: true,

			State: fulfillment.StateUnknown,
		}
		require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecord))

		if state >= commitment.StateReadyToOpen {
			fulfillmentRecord.DisableActiveScheduling = false
			fulfillmentRecord.State = fulfillment.StateConfirmed
			require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))

			paymentRecord := &payment.Record{
				Source:           fulfillmentRecord.Source,
				Destination:      *fulfillmentRecord.Destination,
				BlockId:          e.getNextBlock(),
				TransactionId:    *fulfillmentRecord.Signature,
				TransactionIndex: 2,
			}
			require.NoError(t, e.data.CreatePayment(e.ctx, paymentRecord))
		}
	}
	return commitmentRecords
}

func (e *testEnv) simulateConfirmedAdvances(t *testing.T, commitmentRecords []*commitment.Record) {
	for _, commitmentRecord := range commitmentRecords {
		if commitmentRecord.State != commitment.StateUnknown {
			continue
		}

		commitmentRecord.State = commitment.StateReadyToOpen
		require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))

		fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.TransferWithCommitment, commitmentRecord.Intent, commitmentRecord.ActionId)
		require.NoError(t, err)
		require.Len(t, fulfillmentRecords, 1)
		fulfillmentRecord := fulfillmentRecords[0]

		fulfillmentRecord.State = fulfillment.StateConfirmed
		require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))

		paymentRecord := &payment.Record{
			Source:           fulfillmentRecord.Source,
			Destination:      *fulfillmentRecord.Destination,
			BlockId:          e.getNextBlock(),
			TransactionId:    *fulfillmentRecord.Signature,
			TransactionIndex: 2,
		}
		require.NoError(t, e.data.CreatePayment(e.ctx, paymentRecord))
	}
}

func (e *testEnv) simulateConfirmedIntent(t *testing.T) {
	intentRecord, err := e.data.GetLatestIntentByInitiatorAndType(e.ctx, intent.SaveRecentRoot, e.subsidizer.PublicKey().ToBase58())
	require.NoError(t, err)
	require.Equal(t, intentRecord.IntentType, intent.SaveRecentRoot)
	require.Equal(t, intent.StatePending, intentRecord.State)

	intentRecord.State = intent.StateConfirmed
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	fulfillmentRecords, err := e.data.GetAllFulfillmentsByAction(e.ctx, intentRecord.IntentId, 0)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)
	fulfillmentRecord := fulfillmentRecords[0]

	txnRecord := transaction.Record{
		Signature: *fulfillmentRecord.Signature,
		Slot:      e.getNextBlock(),
	}
	require.NoError(t, e.data.SaveTransaction(e.ctx, &txnRecord))
}

func (e *testEnv) getNextRecentRoot(t *testing.T, commitmentRecords []*commitment.Record) string {
	var leavesToAdd []merkletree.Leaf
	for _, commitmentRecord := range commitmentRecords {
		addressBytes, err := base58.Decode(commitmentRecord.Address)
		require.NoError(t, err)

		leavesToAdd = append(leavesToAdd, addressBytes)
	}

	hash, err := e.merkleTree.SimulateAddingLeaves(e.ctx, leavesToAdd)
	require.NoError(t, err)

	return hex.EncodeToString(hash)
}

func (e *testEnv) assertEmptyMerkleTree(t *testing.T) {
	require.NoError(t, e.merkleTree.Refresh(e.ctx))

	_, err := e.merkleTree.GetLeafNodeByIndex(e.ctx, 0)
	assert.Equal(t, merkletree.ErrLeafNotFound, err)
}

func (e *testEnv) assertIntentNotCreated(t *testing.T) {
	_, err := e.data.GetLatestIntentByInitiatorAndType(e.ctx, intent.SaveRecentRoot, e.subsidizer.PublicKey().ToBase58())
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func (e *testEnv) assertIntentCount(t *testing.T, expected int) {
	actual, err := e.data.GetAllIntentsByOwner(e.ctx, e.subsidizer.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.EqualValues(t, expected, len(actual))
}

func (e *testEnv) assertIntentCreated(t *testing.T) {
	intentRecord, err := e.data.GetLatestIntentByInitiatorAndType(e.ctx, intent.SaveRecentRoot, e.subsidizer.PublicKey().ToBase58())
	require.NoError(t, err)
	require.Equal(t, intentRecord.IntentType, intent.SaveRecentRoot)
	assert.Equal(t, e.subsidizer.PublicKey().ToBase58(), intentRecord.InitiatorOwnerAccount)
	assert.Nil(t, intentRecord.InitiatorPhoneNumber)
	assert.Equal(t, intent.StatePending, intentRecord.State)
	require.NotNil(t, intentRecord.SaveRecentRootMetadata)
	assert.Equal(t, e.treasuryPool.Address, intentRecord.SaveRecentRootMetadata.TreasuryPool)
	assert.Equal(t, e.treasuryPool.GetMostRecentRoot(), intentRecord.SaveRecentRootMetadata.PreviousMostRecentRoot)

	actionRecords, err := e.data.GetAllActionsByIntent(e.ctx, intentRecord.IntentId)
	require.NoError(t, err)
	require.Len(t, actionRecords, 1)
	actionRecord := actionRecords[0]
	assert.Equal(t, intentRecord.IntentId, actionRecord.Intent)
	assert.Equal(t, intent.SaveRecentRoot, actionRecord.IntentType)
	assert.EqualValues(t, 0, actionRecord.ActionId)
	assert.Equal(t, action.SaveRecentRoot, actionRecord.ActionType)
	assert.Equal(t, e.treasuryPool.Vault, actionRecord.Source)
	assert.Nil(t, actionRecord.Destination)
	assert.Nil(t, actionRecord.Quantity)
	assert.Nil(t, actionRecord.InitiatorPhoneNumber)
	assert.Equal(t, action.StatePending, actionRecord.State)

	fulfillmentRecords, err := e.data.GetAllFulfillmentsByIntent(e.ctx, intentRecord.IntentId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)
	fulfillmentRecord := fulfillmentRecords[0]
	assert.Equal(t, fulfillmentRecord.Intent, intentRecord.IntentId)
	assert.Equal(t, intent.SaveRecentRoot, fulfillmentRecord.IntentType)
	assert.EqualValues(t, 0, fulfillmentRecord.ActionId)
	assert.Equal(t, action.SaveRecentRoot, fulfillmentRecord.ActionType)
	assert.Equal(t, fulfillment.SaveRecentRoot, fulfillmentRecord.FulfillmentType)
	assert.Equal(t, e.treasuryPool.Vault, fulfillmentRecord.Source)
	assert.Nil(t, fulfillmentRecord.Destination)
	assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
	assert.EqualValues(t, 0, fulfillmentRecord.ActionOrderingIndex)
	assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
	assert.False(t, fulfillmentRecord.DisableActiveScheduling)
	assert.Nil(t, fulfillmentRecord.InitiatorPhoneNumber)
	assert.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)

	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))
	require.Len(t, txn.Message.Instructions, 2)

	assert.Equal(t, *fulfillmentRecord.Blockhash, base58.Encode(txn.Message.RecentBlockhash[:]))

	expectedSignature := ed25519.Sign(e.subsidizer.PrivateKey().ToBytes(), txn.Message.Marshal())
	assert.Equal(t, base58.Encode(expectedSignature), *fulfillmentRecord.Signature)
	assert.EqualValues(t, txn.Signatures[0][:], expectedSignature)

	advanceNonceIxn, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	assert.Equal(t, *fulfillmentRecord.Nonce, base58.Encode(advanceNonceIxn.Nonce))
	assert.Equal(t, e.subsidizer.PublicKey().ToBase58(), base58.Encode(advanceNonceIxn.Authority))

	saveRecentRootIxnArgs, saveRecentRootIxnAccounts, err := splitter_token.SaveRecentRootInstructionFromLegacyInstruction(txn, 1)
	require.NoError(t, err)
	assert.Equal(t, e.treasuryPool.Bump, saveRecentRootIxnArgs.PoolBump)
	assert.Equal(t, e.treasuryPool.Address, base58.Encode(saveRecentRootIxnAccounts.Pool))
	assert.Equal(t, e.subsidizer.PublicKey().ToBase58(), base58.Encode(saveRecentRootIxnAccounts.Authority))
	assert.Equal(t, e.subsidizer.PublicKey().ToBase58(), base58.Encode(saveRecentRootIxnAccounts.Payer))

	nonceRecord, err := e.data.GetNonce(e.ctx, *fulfillmentRecord.Nonce)
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReserved, nonceRecord.State)
	assert.Equal(t, *fulfillmentRecord.Signature, nonceRecord.Signature)
	assert.Equal(t, nonceRecord.Blockhash, *fulfillmentRecord.Blockhash)
}

func (e *testEnv) assertSyncedMerkleTree(t *testing.T, commitmentRecords []*commitment.Record) {
	require.NoError(t, e.merkleTree.Refresh(e.ctx))

	latestLeafNode, err := e.merkleTree.GetLastAddedLeafNode(e.ctx)
	require.NoError(t, err)
	require.EqualValues(t, latestLeafNode.Index, len(commitmentRecords)-1)

	for i, commitmentRecord := range commitmentRecords {
		leafNode, err := e.merkleTree.GetLeafNodeByIndex(e.ctx, uint64(i))
		require.NoError(t, err)
		assert.Equal(t, commitmentRecord.Address, base58.Encode(leafNode.LeafValue))
	}
}

func (e *testEnv) assertTreasuryAdvancesActiveSchedulingState(t *testing.T, commitmentRecords []*commitment.Record, expected bool) {
	for _, commitmentRecord := range commitmentRecords {
		if commitmentRecord.State != commitment.StateUnknown {
			continue
		}

		fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.TransferWithCommitment, commitmentRecord.Intent, commitmentRecord.ActionId)
		require.NoError(t, err)
		require.Len(t, fulfillmentRecords, 1)
		fulfillmentRecord := fulfillmentRecords[0]
		assert.Equal(t, fulfillmentRecord.DisableActiveScheduling, !expected)
	}
}

func (e *testEnv) generateAvailableNonce(t *testing.T) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	rand.Read(bh[:])

	nonceKey := &vault.Record{
		PublicKey:  nonceAccount.PublicKey().ToBase58(),
		PrivateKey: nonceAccount.PrivateKey().ToBase58(),
		State:      vault.StateAvailable,
		CreatedAt:  time.Now(),
	}
	nonceRecord := &nonce.Record{
		Address:             nonceAccount.PublicKey().ToBase58(),
		Authority:           e.subsidizer.PublicKey().ToBase58(),
		Blockhash:           base58.Encode(bh[:]),
		Environment:         nonce.EnvironmentSolana,
		EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
		Purpose:             nonce.PurposeInternalServerProcess,
		State:               nonce.StateAvailable,
	}
	require.NoError(t, e.data.SaveKey(e.ctx, nonceKey))
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))
	return nonceRecord
}

func (e *testEnv) generateAvailableNonces(t *testing.T, count int) []*nonce.Record {
	var nonces []*nonce.Record
	for i := 0; i < count; i++ {
		nonces = append(nonces, e.generateAvailableNonce(t))
	}
	return nonces
}

func (e *testEnv) getNextBlock() uint64 {
	e.nextBlock += 1
	return e.nextBlock
}
