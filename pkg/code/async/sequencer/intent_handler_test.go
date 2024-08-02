package async_sequencer

// todo: fix tests once sequencer is rounded out for the vm

/*

func TestOpenAccountsIntentHandler_RemainInStatePending(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.OpenAccounts]
	intentRecord := env.createIntent(t, intent.OpenAccounts)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.OpenAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.CloseDormantAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)
}

func TestOpenAccountsIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.OpenAccounts]
	intentRecord := env.createIntent(t, intent.OpenAccounts)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.OpenAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestOpenAccountsIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.OpenAccounts]
	intentRecord := env.createIntent(t, intent.OpenAccounts)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.OpenAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestSendPrivatePaymentIntentHandler_RemainInStatePending(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPrivatePayment]

	intentRecord1 := env.createIntent(t, intent.SendPrivatePayment)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord1.IntentId, action.PrivateTransfer)
	env.confirmAllActionsOfType(t, intentRecord1.IntentId, action.NoPrivacyTransfer)
	env.confirmFirstActionOfType(t, intentRecord1.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)

	intentRecord2 := env.createIntent(t, intent.SendPrivatePayment)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord2.IntentId, action.PrivateTransfer)
	env.confirmFirstActionOfType(t, intentRecord2.IntentId, action.NoPrivacyTransfer)
	env.confirmFirstActionOfType(t, intentRecord2.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)

	intentRecord3 := env.createIntent(t, intent.SendPrivatePayment)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord3.IntentId, action.PrivateTransfer)
	env.confirmAllActionsOfType(t, intentRecord3.IntentId, action.NoPrivacyTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord3.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StatePending)
}

func TestSendPrivatePaymentIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPrivatePayment]
	intentRecord := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.PrivateTransfer)
	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.NoPrivacyTransfer)
	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestSendPrivatePaymentIntentHandler_SchedulerPollingOptimizations(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPrivatePayment]
	intentRecord1 := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))

	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 2, false)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 3, false)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 4, false)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 5, false)

	env.scheduleAllFulfillmentsOfType(t, intentRecord1.IntentId, fulfillment.TransferWithCommitment)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 2, true)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 3, true)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 4, true)
	env.assertSchedulerPollingState(t, intentRecord1.IntentId, 5, true)

	intentRecord2 := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 2, false)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 3, false)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 4, false)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 5, false)

	env.confirmAllFulfillmentsOfType(t, intentRecord2.IntentId, fulfillment.TransferWithCommitment)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 2, true)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 3, true)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 4, true)
	env.assertSchedulerPollingState(t, intentRecord2.IntentId, 5, true)
}

func TestSendPrivatePaymentIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPrivatePayment]

	intentRecord1 := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord1.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StateFailed)

	intentRecord2 := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord2.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StateFailed)

	intentRecord3 := env.createIntent(t, intent.SendPrivatePayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord3.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord3.IntentId, action.NoPrivacyTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord3.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StateFailed)
}

func TestReceivePaymentsPrivatelyIntentHandler_RemainInStatePending(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPrivately]

	intentRecord1 := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord1.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord1.IntentId))
	env.assertIntentState(t, intentRecord1.IntentId, intent.StatePending)

	intentRecord2 := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord2.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord2.IntentId))
	env.assertIntentState(t, intentRecord2.IntentId, intent.StatePending)

	intentRecord3 := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord3.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord3.IntentId, action.CloseEmptyAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord3.IntentId))
	env.assertIntentState(t, intentRecord3.IntentId, intent.StatePending)

	intentRecord4 := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord4.IntentId))
	env.assertIntentState(t, intentRecord4.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord4.IntentId, action.PrivateTransfer)
	env.confirmFirstActionOfType(t, intentRecord4.IntentId, action.CloseEmptyAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord4.IntentId))
	env.assertIntentState(t, intentRecord4.IntentId, intent.StatePending)
}

func TestReceivePaymentsPrivatelyIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPrivately]
	intentRecord := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.PrivateTransfer)
	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.CloseEmptyAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestReceivePaymentsPrivatelyIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPrivately]
	intentRecord := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestReceivePaymentsPrivatelyIntentHandler__SchedulerPollingOptimizations(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPrivately]
	intentRecord := env.createIntent(t, intent.ReceivePaymentsPrivately)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord.IntentId, 2, false)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord.IntentId, 2, false)

	env.confirmAllActionsOfType(t, intentRecord.IntentId, action.PrivateTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)
	env.assertSchedulerPollingState(t, intentRecord.IntentId, 2, true)
}

func TestSaveRecentRootIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SaveRecentRoot]
	intentRecord := env.createIntent(t, intent.SaveRecentRoot)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.SaveRecentRoot)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestSaveRecentRootIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SaveRecentRoot]
	intentRecord := env.createIntent(t, intent.SaveRecentRoot)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.SaveRecentRoot)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestMigrateToPrivacy2022IntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.MigrateToPrivacy2022]
	intentRecord := env.createIntent(t, intent.MigrateToPrivacy2022)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.CloseEmptyAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestMigrateToPrivacy2022IntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.MigrateToPrivacy2022]
	intentRecord := env.createIntent(t, intent.MigrateToPrivacy2022)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.CloseEmptyAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestSendPublicPaymentIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPublicPayment]
	intentRecord := env.createIntent(t, intent.SendPublicPayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.NoPrivacyTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestSendPublicPaymentIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.SendPublicPayment]
	intentRecord := env.createIntent(t, intent.SendPublicPayment)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.NoPrivacyTransfer)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestReceivePaymentsPubliclyIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPublicly]
	intentRecord := env.createIntent(t, intent.ReceivePaymentsPublicly)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestReceivePaymentsPubliclyIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.ReceivePaymentsPublicly]
	intentRecord := env.createIntent(t, intent.ReceivePaymentsPublicly)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.NoPrivacyWithdraw)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

func TestEstablishRelationshipIntentHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.EstablishRelationship]
	intentRecord := env.createIntent(t, intent.EstablishRelationship)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.confirmFirstActionOfType(t, intentRecord.IntentId, action.OpenAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateConfirmed)
}

func TestEstablishRelationshipIntentHandler_TransitionToStateFailed(t *testing.T) {
	env := setupIntentHandlerTestEnv(t)

	intentHandler := env.handlersByType[intent.EstablishRelationship]
	intentRecord := env.createIntent(t, intent.EstablishRelationship)

	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StatePending)

	env.failFirstActionOfType(t, intentRecord.IntentId, action.OpenAccount)
	require.NoError(t, intentHandler.OnActionUpdated(env.ctx, intentRecord.IntentId))
	env.assertIntentState(t, intentRecord.IntentId, intent.StateFailed)
}

type intentHandlerTestEnv struct {
	ctx            context.Context
	data           code_data.Provider
	handlersByType map[intent.Type]IntentHandler
}

func setupIntentHandlerTestEnv(t *testing.T) *intentHandlerTestEnv {
	db := code_data.NewTestDataProvider()
	return &intentHandlerTestEnv{
		ctx:            context.Background(),
		data:           db,
		handlersByType: getIntentHandlers(db),
	}
}

func (e *intentHandlerTestEnv) createIntent(t *testing.T, intentType intent.Type) *intent.Record {
	codeFeeCollector := testutil.NewRandomAccount(t).PublicKey().ToBase58()
	feeAmount := kin.ToQuarks(1)

	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intentType,

		InitiatorOwnerAccount: "owner",

		State: intent.StatePending,
	}

	var actionRecords []*action.Record
	switch intentType {
	case intent.OpenAccounts:
		intentRecord.OpenAccountsMetadata = &intent.OpenAccountsMetadata{}

		for i := 0; i < 3; i++ {
			actionRecords = append(
				actionRecords,
				&action.Record{
					Intent:     intentRecord.IntentId,
					IntentType: intentType,

					ActionId:   2 * uint32(i),
					ActionType: action.OpenAccount,

					Source: fmt.Sprintf("token%d", i),

					State: action.StatePending,
				},
				&action.Record{
					Intent:     intentRecord.IntentId,
					IntentType: intentType,

					ActionId:   2*uint32(i) + 1,
					ActionType: action.CloseDormantAccount,

					Source: fmt.Sprintf("token%d", i),

					State: action.StateUnknown,
				},
			)
		}

	case intent.SendPrivatePayment:
		intentRecord.SendPrivatePaymentMetadata = &intent.SendPrivatePaymentMetadata{
			DestinationTokenAccount: "destination",
			Quantity:                kin.ToQuarks(42),

			ExchangeCurrency: currency.KIN,
			ExchangeRate:     1.0,
			NativeAmount:     42,
			UsdMarketValue:   4.2,

			IsMicroPayment: true,
		}

		tempOutgoing := "temp-outgoing"
		newTempOutgoing := "new-temp-outgoing"
		actionRecords = append(
			actionRecords,
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   0,
				ActionType: action.PrivateTransfer,

				Source:      "bucket",
				Destination: &tempOutgoing,
				Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   1,
				ActionType: action.PrivateTransfer,

				Source:      "bucket",
				Destination: &tempOutgoing,
				Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   2,
				ActionType: action.NoPrivacyTransfer,

				Source:      tempOutgoing,
				Destination: &codeFeeCollector,
				Quantity:    &feeAmount,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   3,
				ActionType: action.NoPrivacyTransfer,

				Source:      tempOutgoing,
				Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
				Quantity:    &feeAmount,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   4,
				ActionType: action.NoPrivacyTransfer,

				Source:      tempOutgoing,
				Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
				Quantity:    &feeAmount,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   5,
				ActionType: action.NoPrivacyWithdraw,

				Source:      tempOutgoing,
				Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
				Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   6,
				ActionType: action.OpenAccount,

				Source: newTempOutgoing,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   7,
				ActionType: action.CloseDormantAccount,

				Source: newTempOutgoing,

				State: action.StateUnknown,
			},
		)

	case intent.ReceivePaymentsPrivately:
		bucket1Kin := "bucket-1-kin"
		bucket10Kin := "bucket-10-kin"
		tempIncoming := "temp-incoming"
		newTempIncoming := "new-temp-incoming"

		amount2Quarks := kin.ToQuarks(2)
		amount10Quarks := kin.ToQuarks(10)
		amount40Quarks := kin.ToQuarks(40)

		intentRecord.ReceivePaymentsPrivatelyMetadata = &intent.ReceivePaymentsPrivatelyMetadata{
			Source:   tempIncoming,
			Quantity: kin.ToQuarks(42),

			UsdMarketValue: 4.2,
		}

		actionRecords = append(
			actionRecords,
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   0,
				ActionType: action.PrivateTransfer,

				Source:      tempIncoming,
				Destination: &bucket1Kin,
				Quantity:    &amount2Quarks,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   1,
				ActionType: action.PrivateTransfer,

				Source:      tempIncoming,
				Destination: &bucket10Kin,
				Quantity:    &amount40Quarks,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   2,
				ActionType: action.CloseEmptyAccount,

				Source: tempIncoming,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   3,
				ActionType: action.PrivateTransfer,

				Source:      bucket1Kin,
				Destination: &bucket10Kin,
				Quantity:    &amount10Quarks,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   4,
				ActionType: action.OpenAccount,

				Source: newTempIncoming,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   5,
				ActionType: action.CloseDormantAccount,

				Source: newTempIncoming,

				State: action.StateUnknown,
			},
		)

	case intent.SaveRecentRoot:
		intentRecord.SaveRecentRootMetadata = &intent.SaveRecentRootMetadata{
			TreasuryPool:           "treasury",
			PreviousMostRecentRoot: "recent-root",
		}

		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentType,

			ActionId:   0,
			ActionType: action.SaveRecentRoot,

			Source: "treasury-vault",

			State: action.StatePending,
		}
		actionRecords = append(actionRecords, actionRecord)

	case intent.MigrateToPrivacy2022:
		intentRecord.MigrateToPrivacy2022Metadata = &intent.MigrateToPrivacy2022Metadata{
			Quantity: 0,
		}

		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentType,

			ActionId:   0,
			ActionType: action.CloseEmptyAccount,

			Source: "legacy-vault",

			State: action.StatePending,
		}
		actionRecords = append(actionRecords, actionRecord)

	case intent.SendPublicPayment:
		intentRecord.SendPublicPaymentMetadata = &intent.SendPublicPaymentMetadata{
			DestinationTokenAccount: "destination",
			Quantity:                kin.ToQuarks(42),

			ExchangeCurrency: currency.KIN,
			ExchangeRate:     1.0,
			NativeAmount:     42,
			UsdMarketValue:   4.2,
		}

		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentType,

			ActionId:   0,
			ActionType: action.NoPrivacyTransfer,

			Source:      "primary",
			Destination: &intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount,

			State: action.StatePending,
		}
		actionRecords = append(actionRecords, actionRecord)
	case intent.ReceivePaymentsPublicly:
		intentRecord.ReceivePaymentsPubliclyMetadata = &intent.ReceivePaymentsPubliclyMetadata{
			Source:       "gift_card",
			Quantity:     kin.ToQuarks(42),
			IsRemoteSend: true,

			OriginalExchangeCurrency: currency.KIN,
			OriginalExchangeRate:     1.0,
			OriginalNativeAmount:     42.0,

			UsdMarketValue: 4.2,
		}

		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentType,

			ActionId:   0,
			ActionType: action.NoPrivacyWithdraw,

			Source:      intentRecord.ReceivePaymentsPubliclyMetadata.Source,
			Destination: pointer.String("temp_incoming"),

			State: action.StatePending,
		}
		actionRecords = append(actionRecords, actionRecord)
	case intent.EstablishRelationship:
		intentRecord.EstablishRelationshipMetadata = &intent.EstablishRelationshipMetadata{
			RelationshipTo: "getcode.com",
		}

		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentType,

			ActionId:   0,
			ActionType: action.OpenAccount,

			Source: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

			State: action.StatePending,
		}
		actionRecords = append(actionRecords, actionRecord)
	default:
		require.Fail(t, "unhandled intent type")
	}

	var fulfillmentRecords []*fulfillment.Record
	var newFulfillmentRecords []*fulfillment.Record
	for _, actionRecord := range actionRecords {
		switch actionRecord.ActionType {
		case action.OpenAccount:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.InitializeLockedTimelockAccount,

				Source: actionRecord.Source,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.CloseEmptyAccount:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.CloseEmptyTimelockAccount,

				Source: actionRecord.Source,

				FulfillmentOrderingIndex: 0,

				DisableActiveScheduling: intentRecord.IntentType == intent.ReceivePaymentsPrivately,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.CloseDormantAccount:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.CloseDormantTimelockAccount,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				IntentOrderingIndex:      uint64(math.MaxInt64),
				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.NoPrivacyTransfer:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,

				DisableActiveScheduling: intentRecord.IntentType == intent.SendPrivatePayment,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.NoPrivacyWithdraw:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.NoPrivacyWithdraw,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,

				DisableActiveScheduling: intentRecord.IntentType == intent.SendPrivatePayment,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.PrivateTransfer:
			treasuryToBob := &fulfillment.Record{
				FulfillmentType: fulfillment.TransferWithCommitment,

				Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,
			}

			aliceToCommitmentWithTemporaryPrivacy := &fulfillment.Record{
				FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,

				Source:      actionRecord.Source,
				Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),

				FulfillmentOrderingIndex: 2000,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, treasuryToBob, aliceToCommitmentWithTemporaryPrivacy)
		case action.SaveRecentRoot:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.SaveRecentRoot,

				Source: actionRecord.Source,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		default:
			assert.Fail(t, "unsupported action type")
		}

		// Fill in remaining fulfillment metadata
		for _, newFulfillmentRecord := range newFulfillmentRecords {
			newFulfillmentRecord.Intent = actionRecord.Intent
			newFulfillmentRecord.IntentType = actionRecord.IntentType

			newFulfillmentRecord.ActionId = actionRecord.ActionId
			newFulfillmentRecord.ActionType = actionRecord.ActionType

			if newFulfillmentRecord.IntentOrderingIndex == 0 {
				newFulfillmentRecord.IntentOrderingIndex = intentRecord.Id
			}
			newFulfillmentRecord.ActionOrderingIndex = actionRecord.ActionId

			newFulfillmentRecord.Data = []byte("data")
			newFulfillmentRecord.Signature = pointer.String(generateRandomSignature())
			newFulfillmentRecord.Nonce = pointer.String(getRandomNonce())
			newFulfillmentRecord.Blockhash = pointer.String("bh")

			newFulfillmentRecord.State = fulfillment.StateUnknown
		}

		fulfillmentRecords = append(fulfillmentRecords, newFulfillmentRecords...)
		newFulfillmentRecords = nil
	}

	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecords...))
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecords...))

	return intentRecord
}

func (e *intentHandlerTestEnv) confirmAllActionsOfType(t *testing.T, intentId string, actionType action.Type) {
	actionRecords, err := e.data.GetAllActionsByIntent(e.ctx, intentId)
	require.NoError(t, err)

	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType == actionType {
			actionRecord.State = action.StateConfirmed
			require.NoError(t, e.data.UpdateAction(e.ctx, actionRecord))
		}
	}
}

func (e *intentHandlerTestEnv) confirmFirstActionOfType(t *testing.T, intentId string, actionType action.Type) {
	actionRecords, err := e.data.GetAllActionsByIntent(e.ctx, intentId)
	require.NoError(t, err)

	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType == actionType {
			actionRecord.State = action.StateConfirmed
			require.NoError(t, e.data.UpdateAction(e.ctx, actionRecord))
			return
		}
	}

	require.Fail(t, "no action updated")
}

func (e *intentHandlerTestEnv) failFirstActionOfType(t *testing.T, intentId string, actionType action.Type) {
	actionRecords, err := e.data.GetAllActionsByIntent(e.ctx, intentId)
	require.NoError(t, err)

	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType == actionType {
			actionRecord.State = action.StateFailed
			require.NoError(t, e.data.UpdateAction(e.ctx, actionRecord))
			return
		}
	}

	require.Fail(t, "no action updated")
}

func (e *intentHandlerTestEnv) confirmAllFulfillmentsOfType(t *testing.T, intentId string, fulfillmentType fulfillment.Type) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByIntent(e.ctx, intentId)
	require.NoError(t, err)

	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillmentType {
			fulfillmentRecord.State = fulfillment.StateConfirmed
			require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
		}
	}
}

func (e *intentHandlerTestEnv) scheduleAllFulfillmentsOfType(t *testing.T, intentId string, fulfillmentType fulfillment.Type) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByIntent(e.ctx, intentId)
	require.NoError(t, err)

	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillmentType {
			fulfillmentRecord.State = fulfillment.StatePending
			require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
		}
	}
}

func (e *intentHandlerTestEnv) assertIntentState(t *testing.T, intentId string, expected intent.State) {
	intentRecord, err := e.data.GetIntent(e.ctx, intentId)
	require.NoError(t, err)
	assert.Equal(t, expected, intentRecord.State)
}

func (e *intentHandlerTestEnv) assertSchedulerPollingState(t *testing.T, intentId string, actionId uint32, expected bool) {
	fulfillmentRecords, err := e.data.GetAllFulfillmentsByAction(e.ctx, intentId, actionId)
	require.NoError(t, err)

	for _, fulfillmentRecord := range fulfillmentRecords {
		assert.Equal(t, expected, !fulfillmentRecord.DisableActiveScheduling)
	}
}

*/
