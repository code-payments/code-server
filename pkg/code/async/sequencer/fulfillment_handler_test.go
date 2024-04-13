package async_sequencer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/solana/system"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

// Note: CanSubmitToBlockchain tests are handled in scheduler testing

func TestInitializeLockedTimelockAccountFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
		Source:          timelockRecord.VaultAddress,
	}

	txnRecord := &transaction.Record{
		Slot: 12345,
	}

	handler := env.handlersByType[fulfillment.InitializeLockedTimelockAccount]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertTimelockRecordInState(t, fulfillmentRecord.Source, timelock_token_v1.StateLocked, txnRecord.Slot)
}

func TestInitializeLockedTimelockAccountFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
	}

	handler := env.handlersByType[fulfillment.InitializeLockedTimelockAccount]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestInitializeLockedTimelockAccountFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
	}

	handler := env.handlersByType[fulfillment.InitializeLockedTimelockAccount]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestInitializeLockedTimelockAccountFulfillmentHandler_OnDemandTransaction(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	env.setupTimelockRecord(t, authority)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.InitializeLockedTimelockAccount]

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
		Source:          timelockAccounts.Vault.PublicKey().ToBase58(),
	}

	env.generateAvailableNonce(t)
	selectedNonce, err := transaction_util.SelectAvailableNonce(env.ctx, env.data, nonce.PurposeOnDemandTransaction)
	require.NoError(t, err)

	assert.True(t, handler.SupportsOnDemandTransactions())
	txn, err := handler.MakeOnDemandTransaction(env.ctx, fulfillmentRecord, selectedNonce)
	require.NoError(t, err)
	env.assertSignedTransaction(t, txn)
	env.assertNoncedTransaction(t, txn, selectedNonce)
	env.assertExpectedInitializeLockedTimelockAccountTransaction(t, txn, authority)
}

func TestCloseEmptyTimelockAccountFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseEmptyTimelockAccount,
		Source:          timelockRecord.VaultAddress,
	}

	txnRecord := &transaction.Record{
		Slot: 12345,
	}

	handler := env.handlersByType[fulfillment.CloseEmptyTimelockAccount]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertTimelockRecordInState(t, fulfillmentRecord.Source, timelock_token_v1.StateClosed, txnRecord.Slot)
}

func TestCloseEmptyTimelockAccountFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseEmptyTimelockAccount,
	}

	handler := env.handlersByType[fulfillment.CloseEmptyTimelockAccount]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestCloseEmptyTimelockAccountFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseEmptyTimelockAccount,
	}

	handler := env.handlersByType[fulfillment.CloseEmptyTimelockAccount]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestCloseDormantTimelockAccountFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseDormantTimelockAccount,
		Source:          timelockRecord.VaultAddress,
	}

	txnRecord := &transaction.Record{
		Slot: 12345,
	}

	handler := env.handlersByType[fulfillment.CloseDormantTimelockAccount]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertTimelockRecordInState(t, fulfillmentRecord.Source, timelock_token_v1.StateClosed, txnRecord.Slot)
}

func TestCloseDormantTimelockAccountFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseDormantTimelockAccount,
	}

	handler := env.handlersByType[fulfillment.CloseDormantTimelockAccount]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestCloseDormantTimelockAccountFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseDormantTimelockAccount,
		Source:          timelockRecord.VaultAddress,
		Intent:          testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		ActionId:        3,
	}

	actionRecord := &action.Record{
		Intent:     fulfillmentRecord.Intent,
		IntentType: intent.OpenAccounts,
		ActionType: action.CloseDormantAccount,
		ActionId:   fulfillmentRecord.ActionId,
		Source:     fulfillmentRecord.Source,
	}
	require.NoError(t, env.data.PutAllActions(env.ctx, actionRecord))

	handler := env.handlersByType[fulfillment.CloseDormantTimelockAccount]

	// Uncomment these tests if we decide to use CloseDormantAccount actions
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnknown,
		timelock_token_v1.StateLocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateUnlocked,
	} {
		timelockRecord.VaultState = state
		timelockRecord.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

		revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
		assert.False(t, revoked)
		assert.False(t, nonceUsed)
		require.NoError(t, err)

		env.assertActionInState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateUnknown)
	}

	timelockRecord.VaultState = timelock_token_v1.StateClosed
	timelockRecord.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.True(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)

	env.assertActionInState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateRevoked)
}

func TestNoPrivacyTransferWithAuthorityFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,
		Source:          timelockRecord.VaultAddress,
	}
	env.setupForPayment(t, fulfillmentRecord)

	txnRecord, err := env.data.GetTransaction(env.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.NoPrivacyTransferWithAuthority]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertPaymentRecordSaved(t, fulfillmentRecord)
}

func TestNoPrivacyTransferWithAuthorityFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,
	}

	handler := env.handlersByType[fulfillment.NoPrivacyTransferWithAuthority]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestNoPrivacyTransferWithAuthorityFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,
	}

	handler := env.handlersByType[fulfillment.NoPrivacyTransferWithAuthority]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestNoPrivacyWithdrawFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	owner := testutil.NewRandomAccount(t)
	timelockRecord := env.setupTimelockRecord(t, owner)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyWithdraw,
		Source:          timelockRecord.VaultAddress,
	}
	env.setupForPayment(t, fulfillmentRecord)

	txnRecord, err := env.data.GetTransaction(env.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.NoPrivacyWithdraw]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertPaymentRecordSaved(t, fulfillmentRecord)
	env.assertTimelockRecordInState(t, fulfillmentRecord.Source, timelock_token_v1.StateClosed, txnRecord.Slot)
}

func TestNoPrivacyWithdrawFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyWithdraw,
	}

	handler := env.handlersByType[fulfillment.NoPrivacyWithdraw]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestNoPrivacyWithdrawFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.NoPrivacyWithdraw,
	}

	handler := env.handlersByType[fulfillment.NoPrivacyWithdraw]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestTemporaryPrivacyTransferWithAuthorityFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,
	}
	env.setupForPayment(t, fulfillmentRecord)

	txnRecord, err := env.data.GetTransaction(env.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.TemporaryPrivacyTransferWithAuthority]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertPaymentRecordSaved(t, fulfillmentRecord)
}

func TestTemporaryPrivacyTransferWithAuthorityFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,
	}

	handler := env.handlersByType[fulfillment.TemporaryPrivacyTransferWithAuthority]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestTemporaryPrivacyTransferWithAuthorityFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	tempPrivacyFulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,
		ActionId:        2,
	}
	env.setupForPayment(t, tempPrivacyFulfillmentRecord)

	handler := env.handlersByType[fulfillment.TemporaryPrivacyTransferWithAuthority]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, tempPrivacyFulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)

	permanentPrivacyFulfillmentRecord := env.simulatePrivacyUpgrade(t, tempPrivacyFulfillmentRecord)

	for _, state := range []fulfillment.State{
		fulfillment.StateUnknown,
		fulfillment.StatePending,
		fulfillment.StateRevoked,
		fulfillment.StateFailed,
		fulfillment.StateConfirmed,
	} {
		permanentPrivacyFulfillmentRecord.State = state
		require.NoError(t, env.data.UpdateFulfillment(env.ctx, permanentPrivacyFulfillmentRecord))

		revoked, nonceUsed, err = handler.IsRevoked(env.ctx, tempPrivacyFulfillmentRecord)
		assert.Equal(t, state == fulfillment.StateConfirmed, revoked)
		assert.Equal(t, state == fulfillment.StateConfirmed, nonceUsed)
		require.NoError(t, err)
	}

	nonceRecord, err := env.data.GetNonce(env.ctx, *tempPrivacyFulfillmentRecord.Nonce)
	require.NoError(t, err)
	nonceRecord.Signature = *tempPrivacyFulfillmentRecord.Signature
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	revoked, nonceUsed, err = handler.IsRevoked(env.ctx, tempPrivacyFulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "too dangerous"))
}

func TestPermanentPrivacyTransferWithAuthorityFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority,
	}
	env.setupForPayment(t, fulfillmentRecord)

	txnRecord, err := env.data.GetTransaction(env.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.PermanentPrivacyTransferWithAuthority]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertPaymentRecordSaved(t, fulfillmentRecord)
}

func TestPermanentPrivacyTransferWithAuthorityFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority,
	}

	handler := env.handlersByType[fulfillment.PermanentPrivacyTransferWithAuthority]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestPermanentPrivacyTransferWithAuthorityFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority,
	}

	handler := env.handlersByType[fulfillment.PermanentPrivacyTransferWithAuthority]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestTransferWithCommitmentFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		Intent:          testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		ActionId:        3,
		FulfillmentType: fulfillment.TransferWithCommitment,
	}
	env.setupForPayment(t, fulfillmentRecord)
	env.setupCommitmentInState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, commitment.StatePayingDestination)

	txnRecord, err := env.data.GetTransaction(env.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	handler := env.handlersByType[fulfillment.TransferWithCommitment]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, txnRecord))
	env.assertPaymentRecordSaved(t, fulfillmentRecord)
	env.assertCommitmentState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, commitment.StateReadyToOpen)
}

func TestTransferWithCommitmentFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TransferWithCommitment,
	}

	handler := env.handlersByType[fulfillment.TransferWithCommitment]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestTransferWithCommitmentFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TransferWithCommitment,
	}

	handler := env.handlersByType[fulfillment.TransferWithCommitment]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestTransferWithCommitmentFulfillmentHandler_OnDemandTransaction(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	intentId := testutil.NewRandomAccount(t).PublicKey().ToBase58()
	commitmentRecord := env.setupCommitmentInState(t, intentId, 6, commitment.StatePayingDestination)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.TransferWithCommitment,
		Source:          testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Destination:     &commitmentRecord.Destination,
		Intent:          intentId,
		ActionId:        6,
	}

	handler := env.handlersByType[fulfillment.TransferWithCommitment]

	env.generateAvailableNonce(t)
	selectedNonce, err := transaction_util.SelectAvailableNonce(env.ctx, env.data, nonce.PurposeOnDemandTransaction)
	require.NoError(t, err)

	assert.True(t, handler.SupportsOnDemandTransactions())
	txn, err := handler.MakeOnDemandTransaction(env.ctx, fulfillmentRecord, selectedNonce)
	require.NoError(t, err)
	env.assertSignedTransaction(t, txn)
	env.assertNoncedTransaction(t, txn, selectedNonce)
	env.assertExpectedTransferWithCommitmentTransaction(t, txn, commitmentRecord, fulfillmentRecord)
}

func TestSaveRecentRootFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.SaveRecentRoot,
	}

	handler := env.handlersByType[fulfillment.SaveRecentRoot]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
}

func TestSaveRecentRootFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.SaveRecentRoot,
	}

	handler := env.handlersByType[fulfillment.SaveRecentRoot]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestSaveRecentRootFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.SaveRecentRoot,
	}

	handler := env.handlersByType[fulfillment.SaveRecentRoot]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestInitializeCommitmentProofFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecords := env.setupOpenCommitmentFulfillments(t)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.InitializeCommitmentProof)

	handler := env.handlersByType[fulfillment.InitializeCommitmentProof]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
}

func TestInitializeCommitmentProofFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.InitializeCommitmentProof]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestInitializeCommitmentProofFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.InitializeCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.InitializeCommitmentProof]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestUploadCommitmentProofFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecords := env.setupOpenCommitmentFulfillments(t)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.UploadCommitmentProof)

	handler := env.handlersByType[fulfillment.UploadCommitmentProof]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
}

func TestUploadCommitmentProofFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.UploadCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.UploadCommitmentProof]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestUploadCommitmentProofFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.UploadCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.UploadCommitmentProof]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestVerifyCommitmentProofFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecords := env.setupOpenCommitmentFulfillments(t)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.VerifyCommitmentProof)

	handler := env.handlersByType[fulfillment.VerifyCommitmentProof]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
}

func TestVerifyCommitmentProofFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.VerifyCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.VerifyCommitmentProof]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TestVerifyCommitmentProofFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.VerifyCommitmentProof,
	}

	handler := env.handlersByType[fulfillment.VerifyCommitmentProof]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestOpenCommitmentVaultFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecords := env.setupOpenCommitmentFulfillments(t)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.OpenCommitmentVault)

	handler := env.handlersByType[fulfillment.OpenCommitmentVault]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
	env.assertCommitmentState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, commitment.StateOpen)
}

func TestOpenCommitmentVaultFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.OpenCommitmentVault,
	}

	handler := env.handlersByType[fulfillment.OpenCommitmentVault]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TesOpenCommitmentVaultFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.OpenCommitmentVault,
	}

	handler := env.handlersByType[fulfillment.OpenCommitmentVault]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestCloseCommitmentVaultFulfillmentHandler_OnSuccess(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		Intent:          testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		ActionId:        3,
		FulfillmentType: fulfillment.CloseCommitmentVault,
	}
	env.setupCommitmentInState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, commitment.StateClosing)

	handler := env.handlersByType[fulfillment.CloseCommitmentVault]

	require.NoError(t, handler.OnSuccess(env.ctx, fulfillmentRecord, nil))
	env.assertCommitmentState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, commitment.StateClosed)
}

func TestCloseCommitmentVaultFulfillmentHandler_OnFailure(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseCommitmentVault,
	}

	handler := env.handlersByType[fulfillment.CloseCommitmentVault]

	recovered, err := handler.OnFailure(env.ctx, fulfillmentRecord, nil)
	assert.False(t, recovered)
	require.NoError(t, err)
}

func TesCloseCommitmentVaultFulfillmentHandler_IsRevoked(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fulfillmentRecord := &fulfillment.Record{
		FulfillmentType: fulfillment.CloseCommitmentVault,
	}

	handler := env.handlersByType[fulfillment.CloseCommitmentVault]

	revoked, nonceUsed, err := handler.IsRevoked(env.ctx, fulfillmentRecord)
	assert.False(t, revoked)
	assert.False(t, nonceUsed)
	require.NoError(t, err)
}

func TestIsTokenAccountOnBlockchain_CodeAccount(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	timelockRecord := &timelock.Record{
		DataVersion:    timelock_token_v1.DataVersion1,
		Address:        "state",
		VaultAddress:   "vault",
		VaultOwner:     "authority",
		VaultState:     timelock_token_v1.StateUnknown,
		TimeAuthority:  "code",
		CloseAuthority: "code",
		Mint:           "mint",
		NumDaysLocked:  timelock_token_v1.DefaultNumDaysLocked,
	}
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnknown,
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateLocked,
		timelock_token_v1.StateClosed,
	} {
		timelockRecord.VaultState = state
		timelockRecord.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

		actual, err := isTokenAccountOnBlockchain(env.ctx, env.data, timelockRecord.VaultAddress)
		require.NoError(t, err)
		assert.Equal(t, timelockRecord.ExistsOnBlockchain(), actual)
	}
}

func TestEstimateTreasuryPoolFundingLevels(t *testing.T) {
	env := setupFulfillmentHandlerTestEnv(t)

	fundingRecord := &treasury.FundingHistoryRecord{
		Vault:         "treasury-pool-vault",
		DeltaQuarks:   int64(kin.ToQuarks(uint64(10))),
		TransactionId: "txn",
		State:         treasury.FundingStateConfirmed,
		CreatedAt:     time.Now(),
	}
	require.NoError(t, env.data.SaveTreasuryPoolFunding(env.ctx, fundingRecord))

	for i := 1; i <= 5; i++ {
		commitmentRecord := &commitment.Record{
			DataVersion: splitter_token.DataVersion1,

			Address: fmt.Sprintf("commitment-%d", i),
			Vault:   fmt.Sprintf("commitment-vault-%d", i),

			Pool:       "treasury-pool",
			RecentRoot: "recent-root",
			Transcript: "transcript",

			Destination: "destination",
			Amount:      kin.ToQuarks(uint64(i)),

			Intent:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			ActionId: 0,

			Owner: "owner",

			TreasuryRepaid: i > 3,
			State:          commitment.StateReadyToOpen,

			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.SaveCommitment(env.ctx, commitmentRecord))
	}

	totalAvailableTreasuryPoolFunds, usedTreasuryPoolFunds, err := estimateTreasuryPoolFundingLevels(env.ctx, env.data, &treasury.Record{
		Address: "treasury-pool",
		Vault:   "treasury-pool-vault",
	})
	require.NoError(t, err)
	assert.EqualValues(t, kin.ToQuarks(10), totalAvailableTreasuryPoolFunds)
	assert.EqualValues(t, kin.ToQuarks(6), usedTreasuryPoolFunds)
}

type fulfillmentHandlerTestEnv struct {
	ctx            context.Context
	data           code_data.Provider
	subsidizer     *common.Account
	handlersByType map[fulfillment.Type]FulfillmentHandler
}

func setupFulfillmentHandlerTestEnv(t *testing.T) *fulfillmentHandlerTestEnv {
	db := code_data.NewTestDataProvider()

	subsidizer := testutil.SetupRandomSubsidizer(t, db)

	require.NoError(t, db.ImportExchangeRates(context.Background(), &currency.MultiRateRecord{
		Time: time.Now(),
		Rates: map[string]float64{
			"usd": 0.1,
		},
	}))

	return &fulfillmentHandlerTestEnv{
		ctx:            context.Background(),
		data:           db,
		subsidizer:     subsidizer,
		handlersByType: getFulfillmentHandlers(db, withManualTestOverrides(&testOverrides{})),
	}
}

func (e *fulfillmentHandlerTestEnv) assertPaymentRecordSaved(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	var transactionIndex int
	switch fulfillmentRecord.FulfillmentType {
	case fulfillment.NoPrivacyWithdraw:
		transactionIndex = 4
	case fulfillment.TemporaryPrivacyTransferWithAuthority,
		fulfillment.PermanentPrivacyTransferWithAuthority,
		fulfillment.TransferWithCommitment,
		fulfillment.NoPrivacyTransferWithAuthority:
		transactionIndex = 2
	default:
		require.Fail(t, "unhandled fulfillment type")
	}

	paymentRecord, err := e.data.GetPayment(e.ctx, *fulfillmentRecord.Signature, transactionIndex)
	require.NoError(t, err)

	actionRecord, err := e.data.GetActionById(e.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	require.NoError(t, err)

	txnRecord, err := e.data.GetTransaction(e.ctx, *fulfillmentRecord.Signature)
	require.NoError(t, err)

	assert.Equal(t, *fulfillmentRecord.Signature, paymentRecord.TransactionId)
	assert.EqualValues(t, transactionIndex, paymentRecord.TransactionIndex)
	assert.Equal(t, fulfillmentRecord.Intent, paymentRecord.Rendezvous)
	assert.Equal(t, fulfillmentRecord.Source, paymentRecord.Source)
	assert.Equal(t, *actionRecord.Quantity, paymentRecord.Quantity)
	assert.Equal(t, *fulfillmentRecord.Destination, paymentRecord.Destination)
	assert.False(t, paymentRecord.IsExternal)
	assert.Equal(t, txnRecord.Slot, paymentRecord.BlockId)
	assert.Equal(t, txnRecord.BlockTime, paymentRecord.BlockTime)
	assert.Equal(t, transaction.ConfirmationFinalized, paymentRecord.ConfirmationState)
}

func (e *fulfillmentHandlerTestEnv) assertTimelockRecordInState(t *testing.T, vault string, state timelock_token_v1.TimelockState, slot uint64) {
	timelockRecord, err := e.data.GetTimelockByVault(e.ctx, vault)
	require.NoError(t, err)
	assert.Equal(t, state, timelockRecord.VaultState)
}

func (e *fulfillmentHandlerTestEnv) setupTimelockRecord(t *testing.T, owner *common.Account) *timelock.Record {
	timelockAccounts, err := owner.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)
	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, e.data.SaveTimelock(e.ctx, timelockRecord))
	return timelockRecord
}

func (e *fulfillmentHandlerTestEnv) setupCommitmentInState(t *testing.T, intentId string, actionId uint32, state commitment.State) *commitment.Record {
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("recent-root%d", mrand.Uint64())))
	recentRoot := hex.EncodeToString(hasher.Sum(nil))

	hasher = sha256.New()
	hasher.Write([]byte(fmt.Sprintf("transcript-%s-%d", intentId, actionId)))
	transcript := hex.EncodeToString(hasher.Sum(nil))

	commitmentRecord := &commitment.Record{
		DataVersion: splitter_token.DataVersion1,

		Address: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Bump:    254,

		Vault:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		VaultBump: 253,

		Pool:       testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		PoolBump:   252,
		RecentRoot: recentRoot,
		Transcript: transcript,

		Intent:   intentId,
		ActionId: actionId,
		Owner:    testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		Destination: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Amount:      kin.ToQuarks(42),

		State: state,
	}
	require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))
	return commitmentRecord
}

func (e *fulfillmentHandlerTestEnv) setupForPayment(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	if fulfillmentRecord.IntentType == intent.UnknownType {
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.TransferWithCommitment, fulfillment.NoPrivacyWithdraw:
			fulfillmentRecord.IntentType = intent.SendPrivatePayment
		case fulfillment.NoPrivacyTransferWithAuthority:
			fulfillmentRecord.IntentType = intent.SendPublicPayment
		default:
			require.Fail(t, "unhandled fulfillment type")
		}
	}

	if fulfillmentRecord.ActionType == action.UnknownType {
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.TransferWithCommitment:
			fulfillmentRecord.ActionType = action.PrivateTransfer
		case fulfillment.NoPrivacyWithdraw:
			fulfillmentRecord.ActionType = action.NoPrivacyWithdraw
		case fulfillment.NoPrivacyTransferWithAuthority:
			fulfillmentRecord.ActionType = action.NoPrivacyTransfer
		default:
			require.Fail(t, "unhandled fulfillment type")
		}
	}

	if len(fulfillmentRecord.Intent) == 0 {
		fulfillmentRecord.Intent = testutil.NewRandomAccount(t).PublicKey().ToBase58()
	}

	if len(fulfillmentRecord.Source) == 0 {
		fulfillmentRecord.Source = testutil.NewRandomAccount(t).PublicKey().ToBase58()
	}

	if fulfillmentRecord.Signature == nil {
		fulfillmentRecord.Signature = pointer.String(fmt.Sprintf("txn%d", mrand.Uint64()))
	}

	if fulfillmentRecord.Destination == nil {
		destination := testutil.NewRandomAccount(t).PublicKey().ToBase58()
		fulfillmentRecord.Destination = &destination
	}

	fulfillmentRecord.Data = []byte("data")

	quantity := mrand.Uint64()
	actionRecord := &action.Record{
		Source:      fulfillmentRecord.Source,
		Destination: fulfillmentRecord.Destination,
		Quantity:    &quantity,

		// We don't care about the below fields (yet)
		Intent:     fulfillmentRecord.Intent,
		IntentType: fulfillmentRecord.IntentType,
		ActionId:   fulfillmentRecord.ActionId,
		ActionType: fulfillmentRecord.ActionType,
		State:      action.StatePending,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecord))

	txnRecord := &transaction.Record{
		Signature:         *fulfillmentRecord.Signature,
		Slot:              12345,
		BlockTime:         time.Now(),
		Data:              []byte("data"),
		HasErrors:         false,
		ConfirmationState: transaction.ConfirmationFinalized,
	}
	require.NoError(t, e.data.SaveTransaction(e.ctx, txnRecord))

	nonceRecord := &nonce.Record{
		Address:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Signature: *fulfillmentRecord.Signature,

		// We don't care about the below fields (yet)
		Authority: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Blockhash: "bh",
		Purpose:   nonce.PurposeClientTransaction,
		State:     nonce.StateReserved,
	}
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))
	fulfillmentRecord.Nonce = pointer.String(nonceRecord.Address)
	fulfillmentRecord.Blockhash = pointer.String(nonceRecord.Blockhash)
}

func (e *fulfillmentHandlerTestEnv) simulatePrivacyUpgrade(t *testing.T, fulfillmentRecord *fulfillment.Record) *fulfillment.Record {
	require.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecord.FulfillmentType)

	cloned := fulfillmentRecord.Clone()
	cloned.Id = 0
	cloned.FulfillmentType = fulfillment.PermanentPrivacyTransferWithAuthority
	cloned.Signature = pointer.String(fmt.Sprintf("txn%d", mrand.Uint64()))
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, &cloned))

	nonceRecord, err := e.data.GetNonce(e.ctx, *fulfillmentRecord.Nonce)
	require.NoError(t, err)

	nonceRecord.Signature = *cloned.Signature
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))

	return &cloned
}

func (e *fulfillmentHandlerTestEnv) assertActionInState(t *testing.T, intentId string, actionId uint32, expected action.State) {
	actionRecord, err := e.data.GetActionById(e.ctx, intentId, actionId)
	require.NoError(t, err)
	assert.Equal(t, expected, actionRecord.State)
}

func (e *fulfillmentHandlerTestEnv) assertCommitmentState(t *testing.T, intentId string, actionId uint32, expected commitment.State) {
	commitmentRecord, err := e.data.GetCommitmentByAction(e.ctx, intentId, actionId)
	require.NoError(t, err)
	assert.Equal(t, expected, commitmentRecord.State)
}

func (e *fulfillmentHandlerTestEnv) setupOpenCommitmentFulfillments(t *testing.T) []*fulfillment.Record {
	intentId := testutil.NewRandomAccount(t)

	commitmentRecord := e.setupCommitmentInState(t, intentId.PublicKey().ToBase58(), 0, commitment.StateOpening)

	fulfillmentRecords := []*fulfillment.Record{
		{
			FulfillmentType: fulfillment.InitializeCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.VerifyCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.OpenCommitmentVault,
		},
		{
			FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,
		},
	}

	for i, fulfillmentRecord := range fulfillmentRecords {
		fulfillmentRecord.Intent = intentId.PublicKey().ToBase58()
		fulfillmentRecord.IntentType = intent.SendPrivatePayment

		fulfillmentRecord.ActionId = 0
		fulfillmentRecord.ActionType = action.PrivateTransfer

		fulfillmentRecord.Data = []byte("data")
		fulfillmentRecord.Signature = pointer.String(generateRandomSignature())

		fulfillmentRecord.Nonce = pointer.String(getRandomNonce())
		fulfillmentRecord.Blockhash = pointer.String("bh")

		if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority {
			fulfillmentRecord.Source = testutil.NewRandomAccount(t).PublicKey().ToBase58()
			fulfillmentRecord.Destination = &commitmentRecord.Vault
		} else {
			fulfillmentRecord.Source = commitmentRecord.Vault
		}

		fulfillmentRecord.IntentOrderingIndex = 12345
		fulfillmentRecord.ActionOrderingIndex = 0
		fulfillmentRecord.FulfillmentOrderingIndex = uint32(i)
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecords...))

	return fulfillmentRecords
}

func (e *fulfillmentHandlerTestEnv) generateAvailableNonce(t *testing.T) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	_, err := rand.Read(bh[:])
	require.NoError(t, err)

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
		Purpose:   nonce.PurposeOnDemandTransaction,
		State:     nonce.StateAvailable,
	}
	require.NoError(t, e.data.SaveKey(e.ctx, nonceKey))
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))
	return nonceRecord
}

func (e *fulfillmentHandlerTestEnv) assertSignedTransaction(t *testing.T, txn *solana.Transaction) {
	assert.EqualValues(t, 1, txn.Message.Header.NumSignatures)
	transactionId := ed25519.Sign(e.subsidizer.PrivateKey().ToBytes(), txn.Message.Marshal())
	assert.EqualValues(t, txn.Signatures[0][:], transactionId)
	assert.Equal(t, base58.Encode(transactionId), base58.Encode(txn.Signature()))
}

func (e *fulfillmentHandlerTestEnv) assertNoncedTransaction(t *testing.T, txn *solana.Transaction, selectedNonce *transaction_util.SelectedNonce) {
	assert.EqualValues(t, txn.Message.RecentBlockhash[:], selectedNonce.Blockhash[:])

	advanceNonceIxn, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	assert.EqualValues(t, advanceNonceIxn.Nonce, selectedNonce.Account.PublicKey().ToBytes())
	assert.EqualValues(t, advanceNonceIxn.Authority, e.subsidizer.PublicKey().ToBytes())
}

func (e *fulfillmentHandlerTestEnv) assertExpectedInitializeLockedTimelockAccountTransaction(t *testing.T, txn *solana.Transaction, authority *common.Account) {
	require.Len(t, txn.Message.Instructions, 2)

	_, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	initializeIxnArgs, initializeIxnAccounts, err := timelock_token_v1.InitializeInstructionFromLegacyInstruction(*txn, 1)
	require.NoError(t, err)

	assert.Equal(t, timelock_token_v1.DefaultNumDaysLocked, initializeIxnArgs.NumDaysLocked)

	assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), initializeIxnAccounts.Timelock)
	assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), initializeIxnAccounts.Vault)
	assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), initializeIxnAccounts.VaultOwner)
	assert.EqualValues(t, kin.TokenMint, initializeIxnAccounts.Mint)
	assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), initializeIxnAccounts.TimeAuthority)
	assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), initializeIxnAccounts.Payer)
}

func (e *fulfillmentHandlerTestEnv) assertExpectedTransferWithCommitmentTransaction(t *testing.T, txn *solana.Transaction, commitmentRecord *commitment.Record, fulfillmentRecord *fulfillment.Record) {
	require.Len(t, txn.Message.Instructions, 3)

	_, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	assertExpectedKreMemoInstruction(t, txn, 1)

	transferIxnArgs, transferIxnAccounts, err := splitter_token.TransferWithCommitmentInstructionFromLegacyInstruction(*txn, 2)
	require.NoError(t, err)

	assert.Equal(t, commitmentRecord.PoolBump, transferIxnArgs.PoolBump)
	assert.Equal(t, commitmentRecord.Amount, transferIxnArgs.Amount)
	assert.Equal(t, commitmentRecord.Transcript, hex.EncodeToString(transferIxnArgs.Transcript))
	assert.Equal(t, commitmentRecord.RecentRoot, hex.EncodeToString(transferIxnArgs.RecentRoot))

	assert.EqualValues(t, commitmentRecord.Pool, base58.Encode(transferIxnAccounts.Pool))
	assert.EqualValues(t, fulfillmentRecord.Source, base58.Encode(transferIxnAccounts.Vault))
	assert.EqualValues(t, commitmentRecord.Destination, base58.Encode(transferIxnAccounts.Destination))
	assert.EqualValues(t, commitmentRecord.Address, base58.Encode(transferIxnAccounts.Commitment))
	assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), transferIxnAccounts.Authority)
	assert.EqualValues(t, e.subsidizer.PublicKey().ToBytes(), transferIxnAccounts.Payer)
}

func assertExpectedKreMemoInstruction(t *testing.T, txn *solana.Transaction, index int) {
	memo, err := memo.DecompileMemo(txn.Message, index)
	require.NoError(t, err)

	kreMemo, err := kin.MemoFromBase64String(string(memo.Data), true)
	require.NoError(t, err)
	assert.EqualValues(t, kin.HighestVersion, kreMemo.Version())
	assert.EqualValues(t, kin.TransactionTypeP2P, kreMemo.TransactionType())
	assert.EqualValues(t, transaction_util.KreAppIndex, kreMemo.AppIndex())
}
