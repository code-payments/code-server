package async_commitment

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"

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
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx          context.Context
	data         code_data.Provider
	treasuryPool *treasury.Record
	merkleTree   *merkletree.MerkleTree
	worker       *service
	subsidizer   *common.Account
}

func setup(t *testing.T) testEnv {
	ctx := context.Background()

	db := code_data.NewTestDataProvider()

	privacyUpgradeCandidateSelectionTimeout = 0

	subsidizer := testutil.SetupRandomSubsidizer(t, db)

	treasuryPoolAddress := testutil.NewRandomAccount(t)
	treasuryPool := &treasury.Record{
		Name: "test-pool",

		Address: treasuryPoolAddress.PublicKey().ToBase58(),
		Bump:    123,

		Vault:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		VaultBump: 100,

		Authority: subsidizer.PublicKey().ToBase58(),

		MerkleTreeLevels: 63,

		CurrentIndex:    1,
		HistoryListSize: 5,

		SolanaBlock: 123,

		State: treasury.TreasuryPoolStateAvailable,
	}

	merkleTree, err := db.InitializeNewMerkleTree(
		ctx,
		treasuryPool.Name,
		treasuryPool.MerkleTreeLevels,
		[]merkletree.Seed{
			cvm.MerkleTreePrefix,
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

	treasuryPoolAddressToName = make(map[string]string)
	cachedMerkleTrees = make(map[string]*refreshingMerkleTree)

	return testEnv{
		ctx:          ctx,
		data:         db,
		treasuryPool: treasuryPool,
		merkleTree:   merkleTree,
		worker:       New(db).(*service),
		subsidizer:   subsidizer,
	}
}

func (e testEnv) simulateCommitment(t *testing.T, recentRoot string, state commitment.State) *commitment.Record {
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

	owner := testutil.NewRandomAccount(t)
	timelockAccounts, err := owner.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	intentRecord := &intent.Record{
		IntentId:   commitmentRecord.Intent,
		IntentType: intent.SendPrivatePayment,

		InitiatorOwnerAccount: owner.PublicKey().ToBase58(),

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Quantity:                kin.ToQuarks(100),

			ExchangeCurrency: currency.KIN,
			ExchangeRate:     1.0,
			NativeAmount:     100,
			UsdMarketValue:   1,
		},

		State: intent.StatePending,
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	actionRecord := &action.Record{
		Intent:     commitmentRecord.Intent,
		IntentType: intentRecord.IntentType,

		ActionId:   commitmentRecord.ActionId,
		ActionType: action.PrivateTransfer,

		Source:      timelockAccounts.Vault.PublicKey().ToBase58(),
		Destination: &commitmentRecord.Destination,

		State: action.StateUnknown,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecord))

	fulfillmentRecord := &fulfillment.Record{
		Intent:     commitmentRecord.Intent,
		IntentType: intentRecord.IntentType,

		ActionId:   commitmentRecord.ActionId,
		ActionType: actionRecord.ActionType,

		FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,
		Data:            []byte("data"),
		Signature:       pointer.String(fmt.Sprintf("sig%d", rand.Uint64())),

		Nonce:     pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		Blockhash: pointer.String("bh"),

		Source:      actionRecord.Source,
		Destination: &e.treasuryPool.Vault,

		State: fulfillment.StateUnknown,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecord))

	timelockRecord := timelockAccounts.ToDBRecord()
	timelockRecord.VaultState = timelock_token_v1.StateLocked
	timelockRecord.Block += 1
	require.NoError(t, e.data.SaveTimelock(e.ctx, timelockRecord))

	return commitmentRecord
}

func (e testEnv) simulateCommitments(t *testing.T, count int, recentRoot string, state commitment.State) []*commitment.Record {
	var commitmentRecords []*commitment.Record
	for i := 0; i < count; i++ {
		commitmentRecords = append(commitmentRecords, e.simulateCommitment(t, recentRoot, state))
	}
	return commitmentRecords
}

func (e testEnv) simulateSourceAccountUnlocked(t *testing.T, commitmentRecord *commitment.Record) {
	actionRecord, err := e.data.GetActionById(e.ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
	require.NoError(t, err)

	timelockRecord, err := e.data.GetTimelockByVault(e.ctx, actionRecord.Source)
	require.NoError(t, err)

	timelockRecord.VaultState = timelock_token_v1.StateUnlocked
	timelockRecord.Block += 1
	require.NoError(t, e.data.SaveTimelock(e.ctx, timelockRecord))
}

func (e testEnv) simulateAddingLeaves(t *testing.T, commitmentRecords []*commitment.Record) {
	for _, commitmentRecord := range commitmentRecords {
		require.True(t, commitmentRecord.State >= commitment.StateReadyToOpen)

		addressBytes, err := base58.Decode(commitmentRecord.Address)
		require.NoError(t, err)

		require.NoError(t, e.merkleTree.AddLeaf(e.ctx, addressBytes))
	}

	require.NoError(t, e.merkleTree.Refresh(e.ctx))

	rootNode, err := e.merkleTree.GetCurrentRootNode(e.ctx)
	require.NoError(t, err)

	e.treasuryPool.CurrentIndex = (e.treasuryPool.CurrentIndex + 1) % e.treasuryPool.HistoryListSize
	e.treasuryPool.HistoryList[e.treasuryPool.CurrentIndex] = hex.EncodeToString(rootNode.Hash)
	e.treasuryPool.SolanaBlock += 1
	require.NoError(t, e.data.SaveTreasuryPool(e.ctx, e.treasuryPool))
}

func (e testEnv) simulateTemporaryPrivacyChequeCashed(t *testing.T, commitmentRecord *commitment.Record, newState fulfillment.State) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, commitmentRecord.Intent, commitmentRecord.ActionId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)

	fulfillmentRecord := fulfillmentRecords[0]
	require.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)
	require.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)
	fulfillmentRecord.State = newState
	require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
}

func (e testEnv) simulatePermanentPrivacyChequeCashed(t *testing.T, commitmentRecord *commitment.Record, newState fulfillment.State) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.PermanentPrivacyTransferWithAuthority, commitmentRecord.Intent, commitmentRecord.ActionId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)

	fulfillmentRecord := fulfillmentRecords[0]
	require.Equal(t, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)
	require.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)
	fulfillmentRecord.State = newState
	require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
}

func (e testEnv) simulateCommitmentBeingUpgraded(t *testing.T, upgradeFrom, upgradeTo *commitment.Record) {
	require.Nil(t, upgradeFrom.RepaymentDivertedTo)

	upgradeFrom.RepaymentDivertedTo = &upgradeTo.Address
	require.NoError(t, e.data.SaveCommitment(e.ctx, upgradeFrom))

	fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, upgradeFrom.Intent, upgradeFrom.ActionId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)
	require.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)

	permanentPrivacyFulfillment := fulfillmentRecords[0].Clone()
	permanentPrivacyFulfillment.Id = 0
	permanentPrivacyFulfillment.Signature = pointer.String(fmt.Sprintf("txn%d", rand.Uint64()))
	permanentPrivacyFulfillment.FulfillmentType = fulfillment.PermanentPrivacyTransferWithAuthority
	permanentPrivacyFulfillment.Destination = &e.treasuryPool.Vault
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, &permanentPrivacyFulfillment))
}

func (e testEnv) assertCommitmentState(t *testing.T, address string, expected commitment.State) {
	commitmentRecord, err := e.data.GetCommitmentByAddress(e.ctx, address)
	require.NoError(t, err)
	assert.Equal(t, expected, commitmentRecord.State)
}

func (e testEnv) assertCommitmentRepaymentStatus(t *testing.T, address string, expected bool) {
	commitmentRecord, err := e.data.GetCommitmentByAddress(e.ctx, address)
	require.NoError(t, err)
	assert.Equal(t, expected, commitmentRecord.TreasuryRepaid)
}
