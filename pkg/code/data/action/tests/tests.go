package tests

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s action.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s action.Store){
		testRoundTrip,
		testBatchPut,
		testGetAllByIntent,
		testGetAllByAddress,
		testGetNetBalance,
		testGetGiftCardClaimedAction,
		testGetGiftCardAutoReturnAction,
		testCountCountFeeActions,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s action.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()

		expected := &action.Record{
			Intent:     "intent",
			IntentType: intent.SendPublicPayment,

			ActionId:   1,
			ActionType: action.NoPrivacyWithdraw,

			Source:      "source",
			Destination: pointer.String("destination"),
			Quantity:    nil,

			FeeType: (*transactionpb.FeePaymentAction_FeeType)(pointer.Int32((int32)(transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL))),

			State: action.StateConfirmed,
		}

		_, err := s.GetById(ctx, expected.Intent, expected.ActionId)
		assert.Equal(t, action.ErrActionNotFound, err)

		assert.Equal(t, action.ErrActionNotFound, s.Update(ctx, expected))

		cloned := expected.Clone()
		require.NoError(t, s.PutAll(ctx, expected))

		actual, err := s.GetById(ctx, expected.Intent, expected.ActionId)
		require.NoError(t, err)

		assert.True(t, expected.Id > 0)
		assert.Equal(t, expected.Id, actual.Id)
		assert.True(t, expected.CreatedAt.After(start))
		assert.Equal(t, expected.CreatedAt, actual.CreatedAt)

		assertEquivalentRecords(t, &cloned, actual)

		assert.Equal(t, action.ErrActionExists, s.PutAll(ctx, expected))

		expected.Quantity = pointer.Uint64(12345)
		expected.State = action.StateFailed
		cloned = expected.Clone()
		require.NoError(t, s.Update(ctx, expected))

		actual, err = s.GetById(ctx, expected.Intent, expected.ActionId)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testBatchPut(t *testing.T, s action.Store) {
	t.Run("testBatchPut", func(t *testing.T) {
		ctx := context.Background()

		//
		// Happy path
		//

		var inserted []*action.Record
		var expected []*action.Record
		for i := 0; i < 1000; i++ {
			actionRecord := &action.Record{
				Intent:      fmt.Sprintf("intent%d", i),
				IntentType:  intent.SendPublicPayment,
				ActionId:    uint32(i),
				ActionType:  action.NoPrivacyTransfer,
				Source:      fmt.Sprintf("source%d", i),
				Destination: pointer.String(fmt.Sprintf("destination%d", i)),
				Quantity:    pointer.Uint64(uint64(i + 1)),
				FeeType:     (*transactionpb.FeePaymentAction_FeeType)(pointer.Int32((int32)(transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL))),
				CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
			}
			cloned := actionRecord.Clone()
			inserted = append(inserted, actionRecord)
			expected = append(expected, &cloned)
		}
		require.NoError(t, s.PutAll(ctx, inserted...))

		for i, actual := range inserted {
			assert.True(t, actual.Id > 0)
			assertEquivalentRecords(t, expected[i], actual)

			fetched, err := s.GetById(ctx, expected[i].Intent, expected[i].ActionId)
			require.NoError(t, err)
			assert.Equal(t, actual.Id, fetched.Id)
			assertEquivalentRecords(t, expected[i], fetched)
		}

		//
		// Insert existing actions
		//

		for _, actionRecord := range inserted {
			cloned := actionRecord.Clone()

			assert.Equal(t, action.ErrActionExists, s.PutAll(ctx, &cloned))

			cloned.Id = 0
			assert.Equal(t, action.ErrActionExists, s.PutAll(ctx, &cloned))
		}

		for i, actual := range inserted {
			fetched, err := s.GetById(ctx, expected[i].Intent, expected[i].ActionId)
			require.NoError(t, err)
			assert.Equal(t, actual.Id, fetched.Id)
			assertEquivalentRecords(t, expected[i], fetched)
		}

		//
		// Insert the same action twice
		//

		actionRecord := &action.Record{
			Intent:     "unique_intent",
			IntentType: intent.OpenAccounts,
			ActionId:   0,
			ActionType: action.OpenAccount,
			Source:     "source",
		}
		assert.Equal(t, action.ErrActionExists, s.PutAll(ctx, actionRecord, actionRecord))

		_, err := s.GetById(ctx, actionRecord.Intent, actionRecord.ActionId)
		assert.Equal(t, action.ErrActionNotFound, err)
	})
}

func testGetAllByIntent(t *testing.T, s action.Store) {
	t.Run("testGetAllByIntent", func(t *testing.T) {
		ctx := context.Background()

		records := []*action.Record{
			{Intent: "i1", IntentType: intent.OpenAccounts, ActionId: 0, ActionType: action.OpenAccount, Source: "source1", State: action.StatePending},
			{Intent: "i1", IntentType: intent.OpenAccounts, ActionId: 1, ActionType: action.CloseDormantAccount, Source: "source1", State: action.StatePending},
			{Intent: "i2", IntentType: intent.OpenAccounts, ActionId: 0, ActionType: action.CloseDormantAccount, Source: "source2", State: action.StateConfirmed},
		}

		require.NoError(t, s.PutAll(ctx, records...))

		actual, err := s.GetAllByIntent(ctx, "i1")
		require.NoError(t, err)

		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[0], actual[0])
		assertEquivalentRecords(t, records[1], actual[1])

		actual, err = s.GetAllByIntent(ctx, "i2")
		require.NoError(t, err)

		require.Len(t, actual, 1)
		assertEquivalentRecords(t, records[2], actual[0])

		_, err = s.GetAllByIntent(ctx, "i3")
		assert.Equal(t, action.ErrActionNotFound, err)
	})
}

func testGetAllByAddress(t *testing.T, s action.Store) {
	t.Run("testGetAllByAddress", func(t *testing.T) {
		ctx := context.Background()

		records := []*action.Record{
			{Intent: "i1", IntentType: intent.OpenAccounts, ActionId: 0, ActionType: action.OpenAccount, Source: "source", State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPrivatePayment, ActionId: 1, ActionType: action.PrivateTransfer, Source: "source", Destination: pointer.String("destination"), State: action.StatePending},
			{Intent: "i2", IntentType: intent.OpenAccounts, ActionId: 0, ActionType: action.CloseDormantAccount, Source: "destination", State: action.StateConfirmed},
		}

		require.NoError(t, s.PutAll(ctx, records...))

		actual, err := s.GetAllByAddress(ctx, "source")
		require.NoError(t, err)

		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[0], actual[0])
		assertEquivalentRecords(t, records[1], actual[1])

		actual, err = s.GetAllByAddress(ctx, "destination")
		require.NoError(t, err)

		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[1], actual[0])
		assertEquivalentRecords(t, records[2], actual[1])

		_, err = s.GetAllByAddress(ctx, "unknown")
		assert.Equal(t, action.ErrActionNotFound, err)
	})
}

func testGetNetBalance(t *testing.T, s action.Store) {
	t.Run("testGetNetBalance", func(t *testing.T) {
		ctx := context.Background()

		for i, actionType := range []action.Type{
			action.NoPrivacyTransfer,
			action.NoPrivacyWithdraw,
			action.PrivateTransfer,
		} {
			source := fmt.Sprintf("source%d", i)
			destination := fmt.Sprintf("destination%d", i)

			var records []*action.Record
			for j, state := range []action.State{
				action.StateUnknown,
				action.StatePending,
				action.StateConfirmed,
				action.StateFailed,
				action.StateRevoked,
			} {
				quantity := uint64(math.Pow10(j))
				paymentAction := &action.Record{
					Intent:     fmt.Sprintf("i%d%d", i, j),
					IntentType: intent.SendPrivatePayment,

					ActionId:   uint32(2 * j),
					ActionType: actionType,

					Source:      source,
					Destination: &destination,
					Quantity:    &quantity,

					State: state,
				}

				closeAccountAction := &action.Record{
					Intent:     fmt.Sprintf("i%d%d", i, j),
					IntentType: intent.SendPrivatePayment,

					ActionId:   uint32(2*j + 1),
					ActionType: action.CloseEmptyAccount,

					Source: source,

					State: state,
				}

				records = append(records, paymentAction, closeAccountAction)
			}

			require.NoError(t, s.PutAll(ctx, records...))

			netBalance, err := s.GetNetBalance(ctx, source)
			require.NoError(t, err)
			assert.EqualValues(t, -1111, netBalance)

			netBalance, err = s.GetNetBalance(ctx, destination)
			require.NoError(t, err)
			assert.EqualValues(t, 1111, netBalance)

			netBalanceByAccount, err := s.GetNetBalanceBatch(ctx, source, destination, destination)
			require.NoError(t, err)
			require.Len(t, netBalanceByAccount, 2)
			assert.EqualValues(t, -1111, netBalanceByAccount[source])
			assert.EqualValues(t, 1111, netBalanceByAccount[destination])
		}

		netBalance, err := s.GetNetBalance(ctx, "unknown")
		require.NoError(t, err)
		assert.EqualValues(t, 0, netBalance)

		netBalanceByAccount, err := s.GetNetBalanceBatch(ctx, "unknown")
		require.NoError(t, err)
		require.Len(t, netBalanceByAccount, 1)
		assert.EqualValues(t, 0, netBalanceByAccount["unknown"])
	})
}

func testGetGiftCardClaimedAction(t *testing.T, s action.Store) {
	t.Run("testGetGiftCardClaimedAction", func(t *testing.T) {
		ctx := context.Background()

		records := []*action.Record{
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 0, ActionType: action.OpenAccount, Source: "a1", State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 1, ActionType: action.PrivateTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 2, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 3, ActionType: action.CloseEmptyAccount, Source: "a1", State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 4, ActionType: action.NoPrivacyWithdraw, Source: "a1", Destination: pointer.String("destination"), State: action.StatePending},
			{Intent: "i1", IntentType: intent.ReceivePaymentsPublicly, ActionId: 5, ActionType: action.CloseDormantAccount, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},

			{Intent: "i2", IntentType: intent.ReceivePaymentsPublicly, ActionId: 1, ActionType: action.NoPrivacyWithdraw, Source: "a2", Destination: pointer.String("destination"), State: action.StateRevoked},
			{Intent: "i2", IntentType: intent.ReceivePaymentsPublicly, ActionId: 2, ActionType: action.NoPrivacyWithdraw, Source: "other", Destination: pointer.String("a2"), State: action.StatePending},
			{Intent: "i2", IntentType: intent.ReceivePaymentsPublicly, ActionId: 0, ActionType: action.NoPrivacyWithdraw, Source: "a2", Destination: pointer.String("destination"), State: action.StatePending},

			{Intent: "i3", IntentType: intent.ReceivePaymentsPublicly, ActionId: 0, ActionType: action.NoPrivacyWithdraw, Source: "a3", Destination: pointer.String("destination"), State: action.StatePending},
			{Intent: "i3", IntentType: intent.ReceivePaymentsPublicly, ActionId: 1, ActionType: action.NoPrivacyWithdraw, Source: "a3", Destination: pointer.String("destination"), State: action.StateConfirmed},
		}

		require.NoError(t, s.PutAll(ctx, records...))

		_, err := s.GetGiftCardClaimedAction(ctx, "unknown")
		assert.Equal(t, action.ErrActionNotFound, err)

		actual, err := s.GetGiftCardClaimedAction(ctx, "a1")
		require.NoError(t, err)
		assertEquivalentRecords(t, records[4], actual)

		actual, err = s.GetGiftCardClaimedAction(ctx, "a2")
		require.NoError(t, err)
		assertEquivalentRecords(t, records[8], actual)

		_, err = s.GetGiftCardClaimedAction(ctx, "a3")
		assert.Equal(t, action.ErrMultipleActionsFound, err)
	})
}

func testGetGiftCardAutoReturnAction(t *testing.T, s action.Store) {
	t.Run("testGetGiftCardAutoReturnAction", func(t *testing.T) {
		ctx := context.Background()

		records := []*action.Record{
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 0, ActionType: action.OpenAccount, Source: "a1", State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 1, ActionType: action.PrivateTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 2, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 3, ActionType: action.CloseEmptyAccount, Source: "a1", State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 4, ActionType: action.CloseDormantAccount, Source: "a1", Destination: pointer.String("destination"), State: action.StatePending},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 5, ActionType: action.NoPrivacyWithdraw, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},

			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 1, ActionType: action.NoPrivacyWithdraw, Source: "a2", Destination: pointer.String("destination"), State: action.StateRevoked},
			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 2, ActionType: action.NoPrivacyWithdraw, Source: "other", Destination: pointer.String("a2"), State: action.StatePending},
			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 0, ActionType: action.NoPrivacyWithdraw, Source: "a2", Destination: pointer.String("destination"), State: action.StatePending},

			{Intent: "i3", IntentType: intent.SendPublicPayment, ActionId: 0, ActionType: action.NoPrivacyWithdraw, Source: "a3", Destination: pointer.String("destination"), State: action.StatePending},
			{Intent: "i3", IntentType: intent.SendPublicPayment, ActionId: 1, ActionType: action.NoPrivacyWithdraw, Source: "a3", Destination: pointer.String("destination"), State: action.StateConfirmed},
		}

		require.NoError(t, s.PutAll(ctx, records...))

		_, err := s.GetGiftCardAutoReturnAction(ctx, "unknown")
		assert.Equal(t, action.ErrActionNotFound, err)

		actual, err := s.GetGiftCardAutoReturnAction(ctx, "a1")
		require.NoError(t, err)
		assertEquivalentRecords(t, records[5], actual)

		actual, err = s.GetGiftCardAutoReturnAction(ctx, "a2")
		require.NoError(t, err)
		assertEquivalentRecords(t, records[8], actual)

		_, err = s.GetGiftCardAutoReturnAction(ctx, "a3")
		assert.Equal(t, action.ErrMultipleActionsFound, err)
	})
}

func testCountCountFeeActions(t *testing.T, s action.Store) {
	t.Run("testCountCountFeeActions", func(t *testing.T) {
		ctx := context.Background()

		feeType := transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL
		records := []*action.Record{
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 0, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StateUnknown},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 1, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StatePending},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 2, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StateFailed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 3, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 4, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},
			{Intent: "i1", IntentType: intent.SendPublicPayment, ActionId: 5, ActionType: action.NoPrivacyTransfer, Source: "a1", Destination: pointer.String("destination"), State: action.StateConfirmed},

			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 1, ActionType: action.NoPrivacyTransfer, Source: "a2", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StateRevoked},
			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 0, ActionType: action.NoPrivacyTransfer, Source: "a2", Destination: pointer.String("destination"), FeeType: &feeType, State: action.StatePending},
			{Intent: "i2", IntentType: intent.SendPublicPayment, ActionId: 2, ActionType: action.NoPrivacyTransfer, Source: "a2", Destination: pointer.String("destination"), State: action.StatePending},
		}

		require.NoError(t, s.PutAll(ctx, records...))

		count, err := s.CountFeeActions(ctx, "i1", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL)
		require.NoError(t, err)
		assert.EqualValues(t, 4, count)

		count, err = s.CountFeeActions(ctx, "i2", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountFeeActions(ctx, "i3", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountFeeActions(ctx, "i1", transactionpb.FeePaymentAction_UNKNOWN)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *action.Record) {
	assert.Equal(t, obj1.Intent, obj2.Intent)
	assert.Equal(t, obj1.IntentType, obj2.IntentType)

	assert.Equal(t, obj1.ActionId, obj2.ActionId)
	assert.Equal(t, obj1.ActionType, obj2.ActionType)

	assert.Equal(t, obj1.Source, obj2.Source)
	assert.EqualValues(t, obj1.Destination, obj2.Destination)
	assert.EqualValues(t, obj1.Quantity, obj2.Quantity)

	assert.EqualValues(t, obj1.FeeType, obj2.FeeType)

	assert.Equal(t, obj1.State, obj2.State)
}
