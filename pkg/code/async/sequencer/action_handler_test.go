package async_sequencer

// todo: fix tests once sequencer is rounded out for the vm

/*

func TestOpenAccountActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.OpenAccount]
	fulfillmentRecords := env.createIntent(t, intent.OpenAccounts)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.InitializeLockedTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TestOpenAccountActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.OpenAccount]
	fulfillmentRecords := env.createIntent(t, intent.OpenAccounts)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.InitializeLockedTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

func TestCloseEmptyAccountActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.CloseEmptyAccount]
	fulfillmentRecords := env.createIntent(t, intent.ReceivePaymentsPrivately)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.CloseEmptyTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TestCloseEmptyAccountActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.CloseEmptyAccount]
	fulfillmentRecords := env.createIntent(t, intent.ReceivePaymentsPrivately)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.CloseEmptyTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

func TesCloseDormantAccountActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.CloseDormantAccount]
	fulfillmentRecords := env.createIntent(t, intent.OpenAccounts)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.CloseDormantTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TesCloseDormantAccountActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.CloseDormantAccount]
	fulfillmentRecords := env.createIntent(t, intent.OpenAccounts)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.CloseDormantTimelockAccount)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

func TestNoPrivacyTransferActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.NoPrivacyTransfer]
	fulfillmentRecords := env.createIntent(t, intent.SendPublicPayment)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.NoPrivacyTransferWithAuthority)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TestNoPrivacyTransferActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.NoPrivacyTransfer]
	fulfillmentRecords := env.createIntent(t, intent.SendPublicPayment)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.NoPrivacyTransferWithAuthority)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

func TestNoPrivacyWithdrawActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.NoPrivacyWithdraw]
	fulfillmentRecords := env.createIntent(t, intent.SendPrivatePayment)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.NoPrivacyWithdraw)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TestNoPrivacyWithdrawActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.NoPrivacyWithdraw]
	fulfillmentRecords := env.createIntent(t, intent.SendPrivatePayment)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.NoPrivacyWithdraw)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

func TestPrivateTransferAction_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)
	handler := env.handlersByType[action.PrivateTransfer]

	for _, fulfillmentType := range []fulfillment.Type{
		fulfillment.TransferWithCommitment,
		fulfillment.TemporaryPrivacyTransferWithAuthority,
		fulfillment.PermanentPrivacyTransferWithAuthority,
	} {
		fulfillmentRecords := env.createIntent(t, intent.SendPrivatePayment)
		fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillmentType)

		require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
		env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

		require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
		if fulfillmentType == fulfillment.TransferWithCommitment {
			env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)
		} else {
			env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
		}
	}
}

func TestPrivateTransferAction_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)
	handler := env.handlersByType[action.PrivateTransfer]

	for _, fulfillmentType := range []fulfillment.Type{
		fulfillment.TransferWithCommitment,
		fulfillment.TemporaryPrivacyTransferWithAuthority,
		fulfillment.PermanentPrivacyTransferWithAuthority,
	} {
		fulfillmentRecords := env.createIntent(t, intent.SendPrivatePayment)
		fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillmentType)

		require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
		env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

		require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
		env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
	}
}

func TestSaveRecentRootActionHandler_TransitionToStateConfirmed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.SaveRecentRoot]
	fulfillmentRecords := env.createIntent(t, intent.SaveRecentRoot)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.SaveRecentRoot)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateConfirmed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateConfirmed)
}

func TestSaveRecentRootActionHandler_TransitionToStateFailed(t *testing.T) {
	env := setupActionHandlerTestEnv(t)

	handler := env.handlersByType[action.SaveRecentRoot]
	fulfillmentRecords := env.createIntent(t, intent.SaveRecentRoot)
	fulfillmentRecord := getFirstFulfillmentOfType(t, fulfillmentRecords, fulfillment.SaveRecentRoot)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StatePending))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StatePending)

	require.NoError(t, handler.OnFulfillmentStateChange(env.ctx, fulfillmentRecord, fulfillment.StateFailed))
	env.assertActionState(t, fulfillmentRecord.Intent, fulfillmentRecord.ActionId, action.StateFailed)
}

type actionHandlerTestEnv struct {
	ctx            context.Context
	data           code_data.Provider
	handlersByType map[action.Type]ActionHandler
}

func setupActionHandlerTestEnv(t *testing.T) actionHandlerTestEnv {
	db := code_data.NewTestDataProvider()
	testutil.SetupRandomSubsidizer(t, db)
	return actionHandlerTestEnv{
		ctx:            context.Background(),
		data:           db,
		handlersByType: getActionHandlers(db),
	}
}

func (e *actionHandlerTestEnv) createIntent(t *testing.T, intentType intent.Type) []*fulfillment.Record {
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
				ActionType: action.NoPrivacyWithdraw,

				Source:      tempOutgoing,
				Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
				Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   2,
				ActionType: action.OpenAccount,

				Source: newTempOutgoing,

				State: action.StatePending,
			},
			&action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentType,

				ActionId:   3,
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
	default:
		require.Fail(t, "unhandled intent type")
	}

	var fulfillmentRecords []*fulfillment.Record
	for _, actionRecord := range actionRecords {
		var newFulfillmentRecords []*fulfillment.Record
		switch actionRecord.ActionType {
		case action.OpenAccount:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.InitializeLockedTimelockAccount,
					Source:          actionRecord.Source,
				},
			)
		case action.CloseEmptyAccount:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.CloseEmptyTimelockAccount,
					Source:          actionRecord.Source,
				},
			)
		case action.CloseDormantAccount:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.CloseDormantTimelockAccount,
					Source:          actionRecord.Source,
					Destination:     actionRecord.Destination,
				},
			)
		case action.NoPrivacyTransfer:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,
					Source:          actionRecord.Source,
					Destination:     actionRecord.Destination,
				},
			)
		case action.NoPrivacyWithdraw:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.NoPrivacyWithdraw,
					Source:          actionRecord.Source,
					Destination:     actionRecord.Destination,
				},
			)
		case action.PrivateTransfer:
			commitmentAddress := fmt.Sprintf("commitment%d", rand.Uint64())
			upgradedToAddress := fmt.Sprintf("commitment%d-vault", rand.Uint64())
			commitmentRecord := &commitment.Record{
				DataVersion: splitter_token.DataVersion1,

				Intent:   actionRecord.Intent,
				ActionId: actionRecord.ActionId,

				Pool:       "treasury",
				RecentRoot: "recent-root",
				Transcript: fmt.Sprintf("transcript%d", rand.Uint64()),

				Address: commitmentAddress,
				Vault:   fmt.Sprintf("%s-vault", commitmentAddress),

				Destination: *actionRecord.Destination,
				Amount:      *actionRecord.Quantity,

				Owner: intentRecord.InitiatorOwnerAccount,

				TreasuryRepaid:      false,
				RepaymentDivertedTo: &upgradedToAddress,

				State: commitment.StateReadyToOpen,
			}
			require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))

			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					Intent:     intentRecord.IntentId,
					IntentType: intentType,

					ActionId:   actionRecord.ActionId,
					ActionType: actionRecord.ActionType,

					FulfillmentType: fulfillment.TransferWithCommitment,

					Source:      "treasury-vault",
					Destination: actionRecord.Destination,

					State: fulfillment.StateUnknown,
				},
				&fulfillment.Record{
					Intent:     intentRecord.IntentId,
					IntentType: intentType,

					ActionId:   actionRecord.ActionId,
					ActionType: actionRecord.ActionType,

					FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,

					Source:      actionRecord.Source,
					Destination: &commitmentRecord.Vault,

					State: fulfillment.StateUnknown,
				},
				&fulfillment.Record{
					Intent:     intentRecord.IntentId,
					IntentType: intentType,

					ActionId:   actionRecord.ActionId,
					ActionType: actionRecord.ActionType,

					FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority,

					Source:      actionRecord.Source,
					Destination: commitmentRecord.RepaymentDivertedTo,

					State: fulfillment.StateUnknown,
				},
			)
		case action.SaveRecentRoot:
			newFulfillmentRecords = append(
				newFulfillmentRecords,
				&fulfillment.Record{
					FulfillmentType: fulfillment.SaveRecentRoot,
					Source:          actionRecord.Source,
				},
			)
		default:
			require.Fail(t, "unhandled action type")
		}

		for _, fulfillmentRecord := range newFulfillmentRecords {
			fulfillmentRecord.Intent = actionRecord.Intent
			fulfillmentRecord.IntentType = actionRecord.IntentType
			fulfillmentRecord.ActionId = actionRecord.ActionId
			fulfillmentRecord.ActionType = actionRecord.ActionType
			fulfillmentRecord.Data = []byte("data")
			fulfillmentRecord.Signature = pointer.String(fmt.Sprintf("txn%d", rand.Uint64()))
			fulfillmentRecord.Nonce = pointer.String(fmt.Sprintf("nonce%d", rand.Uint64()))
			fulfillmentRecord.Blockhash = pointer.String(fmt.Sprintf("bh%d", rand.Uint64()))
		}
		fulfillmentRecords = append(fulfillmentRecords, newFulfillmentRecords...)
	}

	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecords...))
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecords...))

	return fulfillmentRecords
}

func (e *actionHandlerTestEnv) assertActionState(t *testing.T, intentId string, actionId uint32, expected action.State) {
	actionRecord, err := e.data.GetActionById(e.ctx, intentId, actionId)
	require.NoError(t, err)
	assert.Equal(t, expected, actionRecord.State)
}

func getFirstFulfillmentOfType(t *testing.T, records []*fulfillment.Record, fulfillmentType fulfillment.Type) *fulfillment.Record {
	for _, record := range records {
		if record.FulfillmentType == fulfillmentType {
			return record
		}
	}
	require.Fail(t, "fulfillment with type not found")
	return nil
}

*/
