package async_sequencer

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/system"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
)

func TestFulfillmentWorker_StateUnknown_RemainInStateUnknown(t *testing.T) {
	env := setupWorkerEnv(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StateUnknown)

	env.scheduler.shouldSchedule = false
	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateUnknown)
}

func TestFulfillmentWorker_StateUnknown_TransitionToStatePending(t *testing.T) {
	env := setupWorkerEnv(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StateUnknown)

	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateUnknown)

	env.scheduler.shouldSchedule = true
	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
}

func TestFulfillmentWorker_StateUnknown_TransitionToStateRevoked_FulfillmentHandlerMarksAsRevoked(t *testing.T) {
	for _, nonceUsed := range []bool{true, false} {
		env := setupWorkerEnv(t)

		fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StateUnknown)

		require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
		env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateUnknown)

		env.fulfillmentHandler.isRevoked = true
		env.fulfillmentHandler.isNonceUsedWhenRevoked = nonceUsed

		require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
		env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateRevoked)

		if nonceUsed {
			env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateReserved, *fulfillmentRecord.Signature, *fulfillmentRecord.Blockhash)
		} else {
			env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateAvailable, "", *fulfillmentRecord.Blockhash)
		}
	}
}

func TestFulfillmentWorker_StatePending_RemainInPending(t *testing.T) {
	env := setupWorkerEnv(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StatePending)

	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)

	for _, state := range []transaction.Confirmation{
		transaction.ConfirmationUnknown,
		transaction.ConfirmationPending,
		transaction.ConfirmationConfirmed,
	} {
		env.simulateBlockchainTransactionState(t, *fulfillmentRecord.Signature, state)
		require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
		env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
		env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateReserved, *fulfillmentRecord.Signature, *fulfillmentRecord.Blockhash)

		assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
		assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
		assert.False(t, env.actionHandler.callbackExecuted)
		assert.False(t, env.intentHandler.callbackExecuted)
	}
}

func TestFulfillmentWorker_StatePending_OnDemandTransactionCreation_HappyPath(t *testing.T) {
	env := setupWorkerEnv(t)

	env.fulfillmentHandler.supportsOnDemandTxnCreation = true

	nonceRecord := env.generateAvailableNonce(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StatePending)
	fulfillmentRecord.Signature = nil
	fulfillmentRecord.Nonce = nil
	fulfillmentRecord.Blockhash = nil
	fulfillmentRecord.Data = nil
	require.NoError(t, env.data.UpdateFulfillment(env.ctx, fulfillmentRecord))

	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))

	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
	env.assertFulfillmentCreatedOnDemand(t, fulfillmentRecord.Id, nonceRecord.Address, nonceRecord.Blockhash)

	// Should be a no-op in terms of transaction creation
	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))

	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
	env.assertFulfillmentCreatedOnDemand(t, fulfillmentRecord.Id, nonceRecord.Address, nonceRecord.Blockhash)

	assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
	assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
	assert.False(t, env.actionHandler.callbackExecuted)
	assert.False(t, env.intentHandler.callbackExecuted)
}

func TestFulfillmentWorker_StatePending_OnDemandTransactionCreation_NoAvailableNonces(t *testing.T) {
	env := setupWorkerEnv(t)

	env.fulfillmentHandler.supportsOnDemandTxnCreation = true

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StatePending)
	fulfillmentRecord.Signature = nil
	fulfillmentRecord.Nonce = nil
	fulfillmentRecord.Blockhash = nil
	fulfillmentRecord.Data = nil
	require.NoError(t, env.data.UpdateFulfillment(env.ctx, fulfillmentRecord))

	assert.Equal(t, transaction_util.ErrNoAvailableNonces, env.worker.handle(env.ctx, fulfillmentRecord))

	assert.Nil(t, fulfillmentRecord.Signature)
	assert.Nil(t, fulfillmentRecord.Nonce)
	assert.Nil(t, fulfillmentRecord.Blockhash)
	assert.Empty(t, fulfillmentRecord.Data)

	assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
	assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
	assert.False(t, env.actionHandler.callbackExecuted)
	assert.False(t, env.intentHandler.callbackExecuted)
}

func TestFulfillmentWorker_StatePending_TransitionToStateConfirmed(t *testing.T) {
	env := setupWorkerEnv(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StatePending)

	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
	assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
	assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
	assert.False(t, env.actionHandler.callbackExecuted)
	assert.False(t, env.intentHandler.callbackExecuted)

	env.simulateBlockchainTransactionState(t, *fulfillmentRecord.Signature, transaction.ConfirmationFinalized)
	require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
	env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateConfirmed)
	env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateReleased, *fulfillmentRecord.Signature, *fulfillmentRecord.Blockhash)
	assert.True(t, env.fulfillmentHandler.successCallbackExecuted)
	assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
	assert.True(t, env.actionHandler.callbackExecuted)
	assert.Equal(t, fulfillment.StateConfirmed, env.actionHandler.reportedFulfillmentState)
	assert.True(t, env.intentHandler.callbackExecuted)
}

func TestFulfillmentWorker_StatePending_TransitionToStateFailed(t *testing.T) {
	for _, recovered := range []bool{true, false} {
		env := setupWorkerEnv(t)

		fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StatePending)

		require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
		env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
		assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
		assert.False(t, env.fulfillmentHandler.failureCallbackExecuted)
		assert.False(t, env.actionHandler.callbackExecuted)
		assert.False(t, env.intentHandler.callbackExecuted)

		env.fulfillmentHandler.isRecoveredFromFailure = recovered
		env.simulateBlockchainTransactionState(t, *fulfillmentRecord.Signature, transaction.ConfirmationFailed)
		require.NoError(t, env.worker.handle(env.ctx, fulfillmentRecord))
		if recovered {
			env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StatePending)
			env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateReserved, *fulfillmentRecord.Signature, *fulfillmentRecord.Blockhash)
			assert.False(t, env.actionHandler.callbackExecuted)
			assert.False(t, env.intentHandler.callbackExecuted)
		} else {
			env.assertFulfillmentInState(t, *fulfillmentRecord.Signature, fulfillment.StateFailed)
			env.assertNonceState(t, *fulfillmentRecord.Nonce, nonce.StateReleased, *fulfillmentRecord.Signature, *fulfillmentRecord.Blockhash)
			assert.True(t, env.actionHandler.callbackExecuted)
			assert.Equal(t, fulfillment.StateFailed, env.actionHandler.reportedFulfillmentState)
			assert.True(t, env.intentHandler.callbackExecuted)
		}
		assert.False(t, env.fulfillmentHandler.successCallbackExecuted)
		assert.True(t, env.fulfillmentHandler.failureCallbackExecuted)
	}
}

type workerTestEnv struct {
	ctx                context.Context
	data               code_data.Provider
	scheduler          *mockScheduler
	fulfillmentHandler *mockFulfillmentHandler
	actionHandler      *mockActionHandler
	intentHandler      *mockIntentHandler
	worker             *service
	subsidizer         *common.Account
}

func setupWorkerEnv(t *testing.T) *workerTestEnv {
	db := code_data.NewTestDataProvider()

	scheduler := &mockScheduler{}
	fulfillmentHandler := &mockFulfillmentHandler{}
	actionHandler := &mockActionHandler{}
	intentHandler := &mockIntentHandler{}

	worker := New(db, scheduler, withManualTestOverrides(&testOverrides{})).(*service)
	for key := range worker.fulfillmentHandlersByType {
		worker.fulfillmentHandlersByType[key] = fulfillmentHandler
	}
	for key := range worker.actionHandlersByType {
		worker.actionHandlersByType[key] = actionHandler
	}
	for key := range worker.intentHandlersByType {
		worker.intentHandlersByType[key] = intentHandler
	}

	return &workerTestEnv{
		ctx:                context.Background(),
		data:               db,
		scheduler:          scheduler,
		fulfillmentHandler: fulfillmentHandler,
		actionHandler:      actionHandler,
		intentHandler:      intentHandler,
		worker:             worker,
		subsidizer:         testutil.SetupRandomSubsidizer(t, db),
	}
}

func (e *workerTestEnv) createAnyFulfillmentInState(t *testing.T, state fulfillment.State) *fulfillment.Record {
	fakeCodeAccouht := testutil.NewRandomAccount(t)
	fakeNonceAccount := testutil.NewRandomAccount(t)

	txn := solana.NewTransaction(
		fakeCodeAccouht.PublicKey().ToBytes(),
		system.AdvanceNonce(fakeNonceAccount.PublicKey().ToBytes(), fakeCodeAccouht.PublicKey().ToBytes()),
	)

	untypedBlockhash, err := base58.Decode("7EaEajQmomTbW37mzwoQvnw3VLXxhfiR9KxbyKz62ctS")
	require.NoError(t, err)
	var typedBlockhash solana.Blockhash
	copy(typedBlockhash[:], untypedBlockhash)
	txn.SetBlockhash(typedBlockhash)

	txn.Sign(fakeCodeAccouht.PrivateKey().ToBytes())

	fulfillmentRecord := &fulfillment.Record{
		Intent:          testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType:      intent.OpenAccounts,
		ActionId:        3,
		ActionType:      action.OpenAccount,
		FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
		Data:            txn.Marshal(),
		Signature:       pointer.String(base58.Encode(txn.Signature())),
		Source:          "source",
		Nonce:           pointer.String(fakeNonceAccount.PublicKey().ToBase58()),
		Blockhash:       pointer.String(base58.Encode(untypedBlockhash)),
		State:           state,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecord))

	actionRecord := &action.Record{
		Intent:     fulfillmentRecord.Intent,
		IntentType: fulfillmentRecord.IntentType,
		ActionId:   fulfillmentRecord.ActionId,
		ActionType: fulfillmentRecord.ActionType,
		Source:     fulfillmentRecord.Source,
		State:      action.StatePending,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecord))

	nonceRecord := &nonce.Record{
		Address:   *fulfillmentRecord.Nonce,
		Blockhash: *fulfillmentRecord.Blockhash,
		Authority: "code",
		Purpose:   nonce.PurposeClientTransaction,
		Signature: *fulfillmentRecord.Signature,
		State:     nonce.StateReserved,
	}
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))

	return fulfillmentRecord
}

func (e *workerTestEnv) assertFulfillmentInState(t *testing.T, sig string, expected fulfillment.State) {
	fulfillmentRecord, err := e.data.GetFulfillmentBySignature(e.ctx, sig)
	require.NoError(t, err)
	assert.Equal(t, expected, fulfillmentRecord.State)
	switch expected {
	case fulfillment.StateFailed, fulfillment.StateConfirmed, fulfillment.StateRevoked:
		assert.Empty(t, fulfillmentRecord.Data)
	default:
		assert.NotEmpty(t, fulfillmentRecord.Data)
	}
}

func (e *workerTestEnv) assertNonceState(t *testing.T, address string, expectedState nonce.State, expectedSignature, expectedBlockhash string) {
	nonceRecord, err := e.data.GetNonce(e.ctx, address)
	require.NoError(t, err)
	assert.Equal(t, expectedState, nonceRecord.State)
	assert.Equal(t, expectedSignature, nonceRecord.Signature)
	assert.Equal(t, expectedBlockhash, nonceRecord.Blockhash)
}

func (e *workerTestEnv) simulateBlockchainTransactionState(t *testing.T, sig string, state transaction.Confirmation) {
	txnRecord := &transaction.Record{
		Signature:         sig,
		ConfirmationState: state,
	}
	require.NoError(t, e.data.SaveTransaction(e.ctx, txnRecord))
}

func (e *workerTestEnv) assertFulfillmentCreatedOnDemand(t *testing.T, id uint64, nonceAddress, blockhash string) {
	fulfillmentRecord, err := e.data.GetFulfillmentById(e.ctx, id)
	require.NoError(t, err)

	require.NotNil(t, fulfillmentRecord.Signature)
	require.NotNil(t, fulfillmentRecord.Nonce)
	require.NotNil(t, fulfillmentRecord.Blockhash)
	require.NotEmpty(t, fulfillmentRecord.Data)

	expectedTxn := solana.NewTransaction(common.GetSubsidizer().PublicKey().ToBytes(), memo.Instruction(nonceAddress))
	expectedTxn.Sign(e.subsidizer.PrivateKey().ToBytes())
	expectedSignature := base58.Encode(expectedTxn.Signature())

	assert.Equal(t, expectedSignature, *fulfillmentRecord.Signature)
	assert.Equal(t, nonceAddress, *fulfillmentRecord.Nonce)
	assert.Equal(t, blockhash, *fulfillmentRecord.Blockhash)
	assert.Equal(t, expectedTxn.Marshal(), fulfillmentRecord.Data)

	e.assertNonceState(t, nonceAddress, nonce.StateReserved, expectedSignature, blockhash)
}

func (e *workerTestEnv) generateAvailableNonce(t *testing.T) *nonce.Record {
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
		Purpose:   nonce.PurposeOnDemandTransaction,
		State:     nonce.StateAvailable,
	}
	require.NoError(t, e.data.SaveKey(e.ctx, nonceKey))
	require.NoError(t, e.data.SaveNonce(e.ctx, nonceRecord))
	return nonceRecord
}
