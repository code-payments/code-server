package async_commitment

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math"
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
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/solana/system"
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

		SolanaBlock: 123,

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
		DataVersion: splitter_token.DataVersion1,

		Pool:    e.treasuryPool.Address,
		Address: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Vault:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		RecentRoot: recentRoot,
		Transcript: "transcript",

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
		Destination: &commitmentRecord.Vault,

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

	upgradeFrom.RepaymentDivertedTo = &upgradeTo.Vault
	require.NoError(t, e.data.SaveCommitment(e.ctx, upgradeFrom))

	fulfillmentRecords, err := e.data.GetAllFulfillmentsByTypeAndAction(e.ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, upgradeFrom.Intent, upgradeFrom.ActionId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)
	require.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)

	permanentPrivacyFulfillment := fulfillmentRecords[0].Clone()
	permanentPrivacyFulfillment.Id = 0
	permanentPrivacyFulfillment.Signature = pointer.String(fmt.Sprintf("txn%d", rand.Uint64()))
	permanentPrivacyFulfillment.FulfillmentType = fulfillment.PermanentPrivacyTransferWithAuthority
	permanentPrivacyFulfillment.Destination = &upgradeTo.Vault
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, &permanentPrivacyFulfillment))
}

func (e testEnv) assertCommitmentVaultManagementFulfillmentsNotInjected(t *testing.T, commitmentRecord *commitment.Record) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByAction(e.ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
	require.NoError(t, err)
	require.Len(t, fulfillmentRecords, 1)
	assert.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)
}

func (e testEnv) assertCommitmentVaultManagementFulfillmentsInjected(t *testing.T, commitmentRecord *commitment.Record) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByAction(e.ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
	require.NoError(t, err)

	require.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecords[0].FulfillmentType)
	fulfillmentRecords = fulfillmentRecords[1:]

	require.Len(t, fulfillmentRecords, 6)

	poolAddressBytes, err := base58.Decode(e.treasuryPool.Address)
	require.NoError(t, err)

	merkleRootBytes, err := hex.DecodeString(e.treasuryPool.GetMostRecentRoot())
	require.NoError(t, err)

	commitmentAddressBytes, err := base58.Decode(commitmentRecord.Address)
	require.NoError(t, err)

	proofAddressBytes, proofBump, err := splitter_token.GetProofAddress(&splitter_token.GetProofAddressArgs{
		Pool:       poolAddressBytes,
		MerkleRoot: []byte(merkleRootBytes),
		Commitment: commitmentAddressBytes,
	})
	require.NoError(t, err)

	var uploadedProof []merkletree.Hash
	for i, fulfillmentRecord := range fulfillmentRecords {
		//
		// Generic validation
		//

		assert.Equal(t, commitmentRecord.Intent, fulfillmentRecord.Intent)
		assert.Equal(t, intent.SendPrivatePayment, fulfillmentRecord.IntentType)
		assert.EqualValues(t, commitmentRecord.ActionId, fulfillmentRecord.ActionId)
		assert.Equal(t, action.PrivateTransfer, fulfillmentRecord.ActionType)
		assert.Equal(t, commitmentRecord.Vault, fulfillmentRecord.Source)
		assert.Nil(t, fulfillmentRecord.Destination)
		assert.Nil(t, fulfillmentRecord.InitiatorPhoneNumber)
		assert.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)

		nonceRecord, err := e.data.GetNonce(e.ctx, *fulfillmentRecord.Nonce)
		require.NoError(t, err)
		assert.Equal(t, nonce.StateReserved, nonceRecord.State)
		assert.Equal(t, *fulfillmentRecord.Signature, nonceRecord.Signature)
		assert.Equal(t, nonceRecord.Blockhash, *fulfillmentRecord.Blockhash)

		var txn solana.Transaction
		require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

		assert.Equal(t, *fulfillmentRecord.Blockhash, base58.Encode(txn.Message.RecentBlockhash[:]))

		expectedSignature := ed25519.Sign(e.subsidizer.PrivateKey().ToBytes(), txn.Message.Marshal())
		assert.Equal(t, base58.Encode(expectedSignature), *fulfillmentRecord.Signature)
		assert.EqualValues(t, txn.Signatures[0][:], expectedSignature)

		require.NotEmpty(t, txn.Message.Instructions)

		advanceNonceIxn, err := system.DecompileAdvanceNonce(txn.Message, 0)
		require.NoError(t, err)

		assert.Equal(t, *fulfillmentRecord.Nonce, base58.Encode(advanceNonceIxn.Nonce))
		assert.Equal(t, e.subsidizer.PublicKey().ToBase58(), base58.Encode(advanceNonceIxn.Authority))

		//
		// Fulfillment-specific validation based on index
		//

		var expectedFulfillmentType fulfillment.Type
		var expectedIntentOrderingIndex uint64
		expectedFulfillmentOrderingIndex := uint32(i)
		expectedDisableActiveScheduling := true

		switch i {
		case 0:
			expectedFulfillmentType = fulfillment.InitializeCommitmentProof
			expectedDisableActiveScheduling = false

			require.Len(t, txn.Message.Instructions, 2)

			initializeIxnArgs, initializeIxnAccounts, err := splitter_token.InitializeProofInstructionFromLegacyInstruction(txn, 1)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, initializeIxnArgs.PoolBump)
			assert.EqualValues(t, merkleRootBytes, initializeIxnArgs.MerkleRoot)
			assert.EqualValues(t, commitmentAddressBytes, initializeIxnArgs.Commitment)

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(initializeIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, initializeIxnAccounts.Proof)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), initializeIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), initializeIxnAccounts.Payer)
		case 1, 2, 3:
			expectedFulfillmentType = fulfillment.UploadCommitmentProof

			require.Len(t, txn.Message.Instructions, 2)

			uploadIxnArgs, uploadIxnAccounts, err := splitter_token.UploadProofInstructionFromLegacyInstruction(txn, 1)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, uploadIxnArgs.PoolBump)
			assert.Equal(t, proofBump, uploadIxnArgs.ProofBump)
			assert.EqualValues(t, len(uploadedProof), uploadIxnArgs.CurrentSize)
			assert.True(t, uploadIxnArgs.DataSize >= 20)
			assert.True(t, uploadIxnArgs.DataSize <= 22)

			for _, hash := range uploadIxnArgs.Data {
				uploadedProof = append(uploadedProof, merkletree.Hash(hash))
			}

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(uploadIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, uploadIxnAccounts.Proof)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), uploadIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), uploadIxnAccounts.Payer)
		case 4:
			expectedFulfillmentType = fulfillment.OpenCommitmentVault

			require.Len(t, txn.Message.Instructions, 3)

			verifyIxnArgs, verifyIxnAccounts, err := splitter_token.VerifyProofInstructionFromLegacyInstruction(txn, 1)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, verifyIxnArgs.PoolBump)
			assert.Equal(t, proofBump, verifyIxnArgs.ProofBump)

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(verifyIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, verifyIxnAccounts.Proof)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), verifyIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), verifyIxnAccounts.Payer)

			openIxnArgs, openIxnAccounts, err := splitter_token.OpenTokenAccountInstructionFromLegacyInstruction(txn, 2)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, openIxnArgs.PoolBump)
			assert.Equal(t, proofBump, openIxnArgs.ProofBump)

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(openIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, openIxnAccounts.Proof)
			assert.Equal(t, commitmentRecord.Vault, base58.Encode(openIxnAccounts.CommitmentVault))
			assert.EqualValues(t, kin.TokenMint, openIxnAccounts.Mint)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), openIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), openIxnAccounts.Payer)
		case 5:
			expectedFulfillmentType = fulfillment.CloseCommitmentVault

			expectedIntentOrderingIndex = uint64(math.MaxInt64)
			expectedFulfillmentOrderingIndex = 0

			require.Len(t, txn.Message.Instructions, 3)

			closeTokenIxnArgs, closeTokenIxnAccounts, err := splitter_token.CloseTokenAccountInstructionFromLegacyInstruction(txn, 1)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, closeTokenIxnArgs.PoolBump)
			assert.Equal(t, proofBump, closeTokenIxnArgs.ProofBump)
			assert.Equal(t, commitmentRecord.VaultBump, closeTokenIxnArgs.VaultBump)

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(closeTokenIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, closeTokenIxnAccounts.Proof)
			assert.Equal(t, commitmentRecord.Vault, base58.Encode(closeTokenIxnAccounts.CommitmentVault))
			assert.Equal(t, e.treasuryPool.Vault, base58.Encode(closeTokenIxnAccounts.PoolVault))
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), closeTokenIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), closeTokenIxnAccounts.Payer)

			closeProofIxnArgs, closeProofIxnAccounts, err := splitter_token.CloseProofInstructionFromLegacyInstruction(txn, 2)
			require.NoError(t, err)

			assert.Equal(t, e.treasuryPool.Bump, closeProofIxnArgs.PoolBump)
			assert.Equal(t, proofBump, closeProofIxnArgs.ProofBump)

			assert.Equal(t, e.treasuryPool.Address, base58.Encode(closeProofIxnAccounts.Pool))
			assert.EqualValues(t, proofAddressBytes, closeProofIxnAccounts.Proof)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), closeProofIxnAccounts.Authority)
			assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), closeProofIxnAccounts.Payer)
		default:
			assert.Fail(t, "too many fulfillments")
		}

		assert.Equal(t, expectedFulfillmentType, fulfillmentRecord.FulfillmentType)
		assert.Equal(t, expectedIntentOrderingIndex, fulfillmentRecord.IntentOrderingIndex)
		assert.EqualValues(t, 0, fulfillmentRecord.ActionOrderingIndex)
		assert.Equal(t, expectedFulfillmentOrderingIndex, fulfillmentRecord.FulfillmentOrderingIndex)
		assert.Equal(t, expectedDisableActiveScheduling, fulfillmentRecord.DisableActiveScheduling)
	}

	require.Len(t, uploadedProof, int(e.treasuryPool.MerkleTreeLevels))
	assert.True(t, merkletree.Verify(uploadedProof, merkleRootBytes, commitmentAddressBytes))
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
		Address:   nonceAccount.PublicKey().ToBase58(),
		Authority: e.subsidizer.PublicKey().ToBase58(),
		Blockhash: base58.Encode(bh[:]),
		Purpose:   nonce.PurposeInternalServerProcess,
		State:     nonce.StateAvailable,
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
