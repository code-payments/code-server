package tests

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s fulfillment.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s fulfillment.Store){
		testRoundTrip,
		testBatchPut,
		testUpdate,
		testGetAllByState,
		testGetAllByIntent,
		testGetAllByAction,
		testGetAllByTypeAndAction,
		testGetCount,
		testSchedulingQueries,
		testSubsidizerQueries,
		testTreasuryQueries,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s fulfillment.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.GetBySignature(ctx, "test_signature")
		require.Error(t, err)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
		assert.Nil(t, actual)

		expected := fulfillment.Record{
			Signature:                pointer.String("test_signature"),
			Intent:                   "test_intent1",
			IntentType:               intent.SendPrivatePayment,
			ActionId:                 4,
			ActionType:               action.PrivateTransfer,
			FulfillmentType:          fulfillment.TemporaryPrivacyTransferWithAuthority,
			Data:                     []byte("test_data"),
			Nonce:                    pointer.String("test_nonce"),
			Blockhash:                pointer.String("test_blockhash"),
			Source:                   "test_source",
			Destination:              pointer.String("test_destination"),
			IntentOrderingIndex:      1,
			ActionOrderingIndex:      2,
			FulfillmentOrderingIndex: 3,
			DisableActiveScheduling:  false,
			InitiatorPhoneNumber:     pointer.String("+12223334444"),
			State:                    fulfillment.StateConfirmed,
			CreatedAt:                time.Now(),
		}
		cloned := expected.Clone()
		err = s.PutAll(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		actual, err = s.GetBySignature(ctx, "test_signature")
		require.NoError(t, err)
		assert.EqualValues(t, 1, actual.Id)
		assertEquivalentRecords(t, actual, &cloned)

		actual, err = s.GetById(ctx, 2)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
		assert.Nil(t, actual)

		actual, err = s.GetBySignature(ctx, "test_signature_2")
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
		assert.Nil(t, actual)

		assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &expected))
		expected.Id = 0
		assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &expected))

		expected = fulfillment.Record{
			Signature:                nil,
			Intent:                   "test_intent2",
			IntentType:               intent.OpenAccounts,
			ActionId:                 4,
			ActionType:               action.OpenAccount,
			FulfillmentType:          fulfillment.InitializeLockedTimelockAccount,
			Data:                     nil,
			Nonce:                    nil,
			Blockhash:                nil,
			Source:                   "test_source",
			Destination:              nil,
			IntentOrderingIndex:      1,
			ActionOrderingIndex:      2,
			FulfillmentOrderingIndex: 3,
			DisableActiveScheduling:  true,
			InitiatorPhoneNumber:     nil,
			State:                    fulfillment.StateUnknown,
			CreatedAt:                time.Now(),
		}
		cloned = expected.Clone()
		err = s.PutAll(ctx, &expected)
		require.NoError(t, err)
		assert.True(t, expected.Id >= 2)

		actual, err = s.GetById(ctx, expected.Id)
		require.NoError(t, err)
		assert.EqualValues(t, expected.Id, actual.Id)
		assertEquivalentRecords(t, actual, &cloned)

		assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &expected))
	})
}

func testBatchPut(t *testing.T, s fulfillment.Store) {
	t.Run("testBatchPut", func(t *testing.T) {
		ctx := context.Background()

		//
		// Happy path
		//

		var inserted []*fulfillment.Record
		var expected []*fulfillment.Record
		for i := 0; i < 1000; i++ {
			fulfillmentRecord := &fulfillment.Record{
				Signature:                pointer.String(fmt.Sprintf("test_signature%d", i)),
				Intent:                   fmt.Sprintf("test_intent%d", i),
				IntentType:               intent.SendPrivatePayment,
				ActionId:                 uint32(i),
				ActionType:               action.PrivateTransfer,
				FulfillmentType:          fulfillment.TemporaryPrivacyTransferWithAuthority,
				Data:                     []byte(fmt.Sprintf("test_data%d", i)),
				Nonce:                    pointer.String(fmt.Sprintf("test_nonce%d", i)),
				Blockhash:                pointer.String(fmt.Sprintf("test_blockhash%d", i)),
				Source:                   fmt.Sprintf("test_source%d", i),
				Destination:              pointer.String(fmt.Sprintf("test_destination%d", i)),
				IntentOrderingIndex:      uint64(i),
				ActionOrderingIndex:      uint32(i),
				FulfillmentOrderingIndex: uint32(i),
				DisableActiveScheduling:  false,
				InitiatorPhoneNumber:     pointer.String(fmt.Sprintf("+1800555%d", i)),
				State:                    fulfillment.StateConfirmed,
				CreatedAt:                time.Now(),
			}

			cloned := fulfillmentRecord.Clone()
			inserted = append(inserted, fulfillmentRecord)
			expected = append(expected, &cloned)
		}
		require.NoError(t, s.PutAll(ctx, inserted...))

		for i, fulfillmentRecord := range inserted {
			assert.EqualValues(t, i+1, fulfillmentRecord.Id)

			actual, err := s.GetById(ctx, fulfillmentRecord.Id)
			require.NoError(t, err)
			assertEquivalentRecords(t, expected[i], actual)

			actual, err = s.GetBySignature(ctx, *fulfillmentRecord.Signature)
			require.NoError(t, err)
			assertEquivalentRecords(t, expected[i], actual)
		}

		//
		// Re-insert the same signature
		//

		for _, fulfillmentRecord := range inserted {
			cloned := fulfillmentRecord.Clone()
			assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &cloned))

			cloned.Id = 0
			cloned.Source = "something_else"
			cloned.Destination = pointer.String("something_else")
			cloned.Data = []byte("something_else")
			cloned.State = fulfillment.StateFailed
			assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &cloned))
		}
		assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, inserted...))

		for i, fulfillmentRecord := range expected {
			actual, err := s.GetBySignature(ctx, *fulfillmentRecord.Signature)
			require.NoError(t, err)
			assert.Equal(t, inserted[i].Id, actual.Id)
			assertEquivalentRecords(t, fulfillmentRecord, actual)
		}

		//
		// Insert new fulfillments with the same signature
		//

		fulfillmentRecord := expected[0].Clone()
		fulfillmentRecord.Id = 0
		fulfillmentRecord.Signature = pointer.String("unique_signature")
		assert.Equal(t, fulfillment.ErrFulfillmentExists, s.PutAll(ctx, &fulfillmentRecord, &fulfillmentRecord))

		_, err := s.GetBySignature(ctx, *fulfillmentRecord.Signature)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
	})
}

func testUpdate(t *testing.T, s fulfillment.Store) {
	t.Run("testUpdate", func(t *testing.T) {
		ctx := context.Background()

		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, s.MarkAsActivelyScheduled(ctx, 1))

		expected := fulfillment.Record{
			Intent:                   "test_intent",
			IntentType:               intent.OpenAccounts,
			ActionId:                 4,
			ActionType:               action.CloseDormantAccount,
			FulfillmentType:          fulfillment.CloseDormantTimelockAccount,
			Data:                     nil,
			Signature:                nil,
			Nonce:                    nil,
			Blockhash:                nil,
			Source:                   "test_source",
			Destination:              nil,
			InitiatorPhoneNumber:     pointer.String("+12223334444"),
			IntentOrderingIndex:      1,
			ActionOrderingIndex:      2,
			FulfillmentOrderingIndex: 3,
			DisableActiveScheduling:  true,
			State:                    fulfillment.StateUnknown,
			CreatedAt:                time.Now(),
		}

		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, s.Update(ctx, &expected))

		err := s.PutAll(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		require.NoError(t, s.MarkAsActivelyScheduled(ctx, 1))
		actual, err := s.GetById(ctx, 1)
		require.NoError(t, err)
		assert.False(t, actual.DisableActiveScheduling)
		expected.DisableActiveScheduling = false

		expected.State = fulfillment.StatePending
		cloned := expected.Clone()
		err = s.Update(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		actual, err = s.GetById(ctx, 1)
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)

		expected.Signature = pointer.String("test_signature")
		expected.Nonce = pointer.String("test_nonce")
		expected.Blockhash = pointer.String("test_blockhash")
		expected.Data = []byte("test_data")
		cloned = expected.Clone()
		err = s.Update(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		actual, err = s.GetBySignature(ctx, "test_signature")
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)

		expected.IntentOrderingIndex = math.MaxInt64
		expected.ActionOrderingIndex = math.MaxInt32
		expected.FulfillmentOrderingIndex = math.MaxInt32 - 1

		actual, err = s.GetBySignature(ctx, "test_signature")
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)

		expected.Data = nil
		expected.State = fulfillment.StateConfirmed
		cloned = expected.Clone()
		err = s.Update(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		actual, err = s.GetBySignature(ctx, "test_signature")
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)

		expected.Id = 100
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, s.Update(ctx, &expected))
	})
}

func testGetAllByState(t *testing.T, s fulfillment.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		expected := []*fulfillment.Record{
			{Signature: pointer.String("t1"), State: fulfillment.StatePending},
			{Signature: pointer.String("t2"), State: fulfillment.StateRevoked},
			{Signature: pointer.String("t3"), State: fulfillment.StateUnknown},
			{Signature: pointer.String("t4"), State: fulfillment.StateUnknown, DisableActiveScheduling: true},
			{Signature: pointer.String("t5"), State: fulfillment.StateUnknown, DisableActiveScheduling: true},
			{Signature: pointer.String("t6"), State: fulfillment.StateRevoked},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range expected {
			record.IntentType = intent.SendPrivatePayment
			record.Intent = fmt.Sprintf("i%d", i%3+1)
			record.ActionType = action.PrivateTransfer
			record.FulfillmentType = fulfillment.TemporaryPrivacyTransferWithAuthority
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
			record.Source = "test_source"
			record.Destination = pointer.String("test_destination")
		}

		err := s.PutAll(ctx, expected...)
		require.NoError(t, err)

		// Simple get all by state
		actual, err := s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, false, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByState(ctx, fulfillment.StatePending, true, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByState(ctx, fulfillment.StateRevoked, true, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Simple get all by state (reverse)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByState(ctx, fulfillment.StatePending, true, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByState(ctx, fulfillment.StateRevoked, true, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Check items (asc)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t5", *actual[2].Signature)

		// Check items (desc)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t3", *actual[2].Signature)

		// Check items (asc + limit)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 2, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)

		// Check items (desc + limit)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.EmptyCursor, 2, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.ToCursor(1), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t5", *actual[2].Signature)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.ToCursor(6), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t3", *actual[2].Signature)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.ToCursor(3), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t4", *actual[0].Signature)
		assert.Equal(t, "t5", *actual[1].Signature)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.ToCursor(4), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)

		// Check items (asc + cursor + limit)
		actual, err = s.GetAllByState(ctx, fulfillment.StateUnknown, true, query.ToCursor(3), 1, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t4", *actual[0].Signature)
	})
}

func testGetAllByIntent(t *testing.T, s fulfillment.Store) {
	t.Run("testGetAllByIntent", func(t *testing.T) {
		ctx := context.Background()

		expected := []*fulfillment.Record{
			{Signature: pointer.String("t1"), Intent: "i1"},
			{Signature: pointer.String("t2"), Intent: "i2"},
			{Signature: pointer.String("t3"), Intent: "i0"},
			{Signature: pointer.String("t4"), Intent: "i0"},
			{Signature: pointer.String("t5"), Intent: "i0"},
			{Signature: pointer.String("t6"), Intent: "i2"},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range expected {
			record.IntentType = intent.SendPrivatePayment
			record.ActionType = action.PrivateTransfer
			record.FulfillmentType = fulfillment.TemporaryPrivacyTransferWithAuthority
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
			record.Source = "test_source"
			record.Destination = pointer.String("test_destination")
		}

		err := s.PutAll(ctx, expected...)
		require.NoError(t, err)

		// Simple get all by state
		actual, err := s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByIntent(ctx, "i1", query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByIntent(ctx, "i2", query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Simple get all by state (reverse)
		actual, err = s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByIntent(ctx, "i1", query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByIntent(ctx, "i2", query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Check items (asc)
		actual, err = s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t5", *actual[2].Signature)

		// Check items (desc)
		actual, err = s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t3", *actual[2].Signature)

		// Check items (asc + limit)
		actual, err = s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 2, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)

		// Check items (desc + limit)
		actual, err = s.GetAllByIntent(ctx, "i0", query.EmptyCursor, 2, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)

		// Check items (asc + cursor)
		actual, err = s.GetAllByIntent(ctx, "i0", query.ToCursor(1), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t5", *actual[2].Signature)

		// Check items (desc + cursor)
		actual, err = s.GetAllByIntent(ctx, "i0", query.ToCursor(6), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", *actual[0].Signature)
		assert.Equal(t, "t4", *actual[1].Signature)
		assert.Equal(t, "t3", *actual[2].Signature)

		// Check items (asc + cursor)
		actual, err = s.GetAllByIntent(ctx, "i0", query.ToCursor(3), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t4", *actual[0].Signature)
		assert.Equal(t, "t5", *actual[1].Signature)

		// Check items (desc + cursor)
		actual, err = s.GetAllByIntent(ctx, "i0", query.ToCursor(4), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t3", *actual[0].Signature)

		// Check items (asc + cursor + limit)
		actual, err = s.GetAllByIntent(ctx, "i0", query.ToCursor(3), 1, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t4", *actual[0].Signature)
	})
}

func testGetAllByAction(t *testing.T, s fulfillment.Store) {
	t.Run("testGetAllByAction", func(t *testing.T) {
		ctx := context.Background()

		expected := []*fulfillment.Record{
			{Signature: pointer.String("t1"), Intent: "i1", ActionId: 0},
			{Signature: pointer.String("t2"), Intent: "i1", ActionId: 0},
			{Signature: pointer.String("t3"), Intent: "i1", ActionId: 1},
			{Signature: pointer.String("t4"), Intent: "i2", ActionId: 0},
			{Signature: pointer.String("t5"), Intent: "i2", ActionId: 0},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range expected {
			record.IntentType = intent.SendPrivatePayment
			record.ActionType = action.PrivateTransfer
			record.FulfillmentType = fulfillment.TemporaryPrivacyTransferWithAuthority
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
			record.Source = "test_source"
			record.Destination = pointer.String("test_destination")
		}

		err := s.PutAll(ctx, expected...)
		require.NoError(t, err)

		actual, err := s.GetAllByAction(ctx, "i1", 0)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "t1", *actual[0].Signature)
		assert.Equal(t, "t2", *actual[1].Signature)

		actual, err = s.GetAllByAction(ctx, "i1", 1)
		require.NoError(t, err)
		require.Len(t, actual, 1)
		assert.Equal(t, "t3", *actual[0].Signature)

		_, err = s.GetAllByAction(ctx, "i3", 0)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
	})
}

func testGetAllByTypeAndAction(t *testing.T, s fulfillment.Store) {
	t.Run("testGetAllByTypeAndAction", func(t *testing.T) {
		ctx := context.Background()

		expected := []*fulfillment.Record{
			{Signature: pointer.String("t1"), Intent: "i1", ActionId: 0, FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority},
			{Signature: pointer.String("t2"), Intent: "i1", ActionId: 0, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority},
			{Signature: pointer.String("t3"), Intent: "i1", ActionId: 1, FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority},
			{Signature: pointer.String("t4"), Intent: "i1", ActionId: 2, FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority},
			{Signature: pointer.String("t5"), Intent: "i1", ActionId: 2, FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range expected {
			record.IntentType = intent.SendPrivatePayment
			record.ActionType = action.PrivateTransfer
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
			record.Source = "test_source"
			record.Destination = pointer.String("test_destination")
		}

		err := s.PutAll(ctx, expected...)
		require.NoError(t, err)

		actual, err := s.GetAllByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, "i1", 0)
		require.NoError(t, err)
		require.Len(t, actual, 1)
		assert.Equal(t, "t1", *actual[0].Signature)

		actual, err = s.GetAllByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, "i1", 2)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "t4", *actual[0].Signature)
		assert.Equal(t, "t5", *actual[1].Signature)

		_, err = s.GetAllByTypeAndAction(ctx, fulfillment.PermanentPrivacyTransferWithAuthority, "i1", 2)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
	})
}

func testGetCount(t *testing.T, s fulfillment.Store) {
	t.Run("testGetCount", func(t *testing.T) {
		ctx := context.Background()

		expected := []*fulfillment.Record{
			{Intent: "i1", State: fulfillment.StatePending, Source: "s1", Destination: pointer.String("destination"), ActionOrderingIndex: 0},
			{Intent: "i2", State: fulfillment.StateRevoked, Source: "s1", Destination: pointer.String("destination"), ActionOrderingIndex: 0},
			{Intent: "i0", State: fulfillment.StateUnknown, Source: "s1", Destination: pointer.String("destination"), ActionOrderingIndex: 0},
			{Intent: "i0", State: fulfillment.StateUnknown, Source: "s1", Destination: pointer.String("destination"), ActionOrderingIndex: 0},
			{Intent: "i0", ActionId: 1, State: fulfillment.StateUnknown, Source: "s2", Destination: pointer.String("destination"), ActionOrderingIndex: 1},
			{Intent: "i2", State: fulfillment.StateRevoked, Source: "s1", Destination: pointer.String("destination"), ActionOrderingIndex: 0},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range expected {
			record.IntentType = intent.SendPrivatePayment
			record.ActionType = action.PrivateTransfer
			record.FulfillmentType = fulfillment.TemporaryPrivacyTransferWithAuthority
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Signature = pointer.String(fmt.Sprintf("t%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
		}

		for index, item := range expected {
			count, err := s.Count(ctx)
			require.NoError(t, err)
			assert.EqualValues(t, index, count)

			err = s.PutAll(ctx, item)
			require.NoError(t, err)
		}

		count, err := s.CountByState(ctx, fulfillment.StateConfirmed)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByState(ctx, fulfillment.StatePending)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByState(ctx, fulfillment.StateRevoked)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByState(ctx, fulfillment.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByStateAndAddress(ctx, fulfillment.StateUnknown, "s1")
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByStateAndAddress(ctx, fulfillment.StatePending, "destination")
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByStateAndAddress(ctx, fulfillment.StateUnknown, "unknown")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByTypeStateAndAddress(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown, "s2")
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByTypeStateAndAddress(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown, "destination")
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByTypeStateAndAddress(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StatePending, "s2")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByTypeStateAndAddress(ctx, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.StateUnknown, "s2")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByTypeStateAndAddress(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown, "unknown")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByTypeStateAndAddressAsSource(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown, "s2")
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByTypeStateAndAddressAsSource(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown, "destination")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByIntentAndState(ctx, "i0", fulfillment.StateConfirmed)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByIntentAndState(ctx, "i1", fulfillment.StatePending)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByIntentAndState(ctx, "i2", fulfillment.StateRevoked)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByIntentAndState(ctx, "i0", fulfillment.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByIntent(ctx, "i3")
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByIntent(ctx, "i1")
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByIntent(ctx, "i2")
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByIntent(ctx, "i0")
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByTypeActionAndState(ctx, "i0", 0, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByTypeActionAndState(ctx, "i0", 1, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByTypeActionAndState(ctx, "i2", 0, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateRevoked)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByTypeActionAndState(ctx, "i2", 0, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.StateRevoked)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByTypeActionAndState(ctx, "i2", 0, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StatePending)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		countByType, err := s.CountByStateGroupedByType(ctx, fulfillment.StateUnknown)
		require.NoError(t, err)
		assert.Len(t, countByType, 1)
		assert.EqualValues(t, 3, countByType[fulfillment.TemporaryPrivacyTransferWithAuthority])
	})
}

func testSchedulingQueries(t *testing.T, s fulfillment.Store) {
	t.Run("testSchedulingQueries", func(t *testing.T) {
		ctx := context.Background()

		account1 := "test_account1"
		account2 := "test_account2"

		var records []*fulfillment.Record
		for i := 0; i < 5; i++ {
			for j := 0; j < 3; j++ {
				for k := 0; k < 3; k++ {
					source := account1
					destination := account2
					fulfillmentType := fulfillment.TemporaryPrivacyTransferWithAuthority
					state := fulfillment.StateUnknown
					if i%2 != 0 {
						source = account2
						destination = account1
						fulfillmentType = fulfillment.PermanentPrivacyTransferWithAuthority
						state = fulfillment.StatePending
					}

					record := &fulfillment.Record{
						Intent:     fmt.Sprintf("i%d", i),
						IntentType: intent.SendPrivatePayment,

						ActionType: action.PrivateTransfer,

						FulfillmentType: fulfillmentType,
						Data:            []byte("data"),
						Signature:       pointer.String(fmt.Sprintf("t%d%d%d", i, j, k)),

						Nonce:     pointer.String(fmt.Sprintf("n%d%d%d", i, j, k)),
						Blockhash: pointer.String(fmt.Sprintf("bh%d%d%d", i, j, k)),

						Source:      source,
						Destination: &destination,

						IntentOrderingIndex:      uint64(i),
						ActionOrderingIndex:      uint32(j),
						FulfillmentOrderingIndex: uint32(k),

						State: state,
					}

					records = append(records, record)
				}
			}
		}

		for i := len(records) - 1; i >= 0; i-- {
			require.NoError(t, s.PutAll(ctx, records[i]))
		}

		_, err := s.GetFirstSchedulableByAddressAsSource(ctx, "other_account")
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)

		_, err = s.GetFirstSchedulableByAddressAsDestination(ctx, "other_account")
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)

		_, err = s.GetNextSchedulableByAddress(ctx, "other_account", 0, 0, 0)
		assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)

		actual, err := s.GetFirstSchedulableByAddressAsSource(ctx, account1)
		require.NoError(t, err)
		assert.Equal(t, "t000", *actual.Signature)

		actual, err = s.GetFirstSchedulableByAddressAsDestination(ctx, account1)
		require.NoError(t, err)
		assert.Equal(t, "t100", *actual.Signature)

		actual, err = s.GetFirstSchedulableByAddressAsSource(ctx, account2)
		require.NoError(t, err)
		assert.Equal(t, "t100", *actual.Signature)

		actual, err = s.GetFirstSchedulableByAddressAsDestination(ctx, account2)
		require.NoError(t, err)
		assert.Equal(t, "t000", *actual.Signature)

		actual, err = s.GetFirstSchedulableByType(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority)
		require.NoError(t, err)
		assert.Equal(t, "t000", *actual.Signature)

		actual, err = s.GetFirstSchedulableByType(ctx, fulfillment.PermanentPrivacyTransferWithAuthority)
		require.NoError(t, err)
		assert.Equal(t, "t100", *actual.Signature)

		for i, record := range records {
			for _, account := range []string{account1, account2} {
				actual, err = s.GetNextSchedulableByAddress(ctx, account, record.IntentOrderingIndex, record.ActionOrderingIndex, record.FulfillmentOrderingIndex)
				if i == len(records)-1 {
					assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, *records[i+1].Signature, *actual.Signature)
				}
			}
		}
	})
}

func testSubsidizerQueries(t *testing.T, s fulfillment.Store) {
	t.Run("testSubsidizerQueries", func(t *testing.T) {
		ctx := context.Background()

		records := []*fulfillment.Record{
			// Included in counts
			{State: fulfillment.StatePending, ActionType: action.OpenAccount, FulfillmentType: fulfillment.InitializeLockedTimelockAccount},
			{State: fulfillment.StatePending, ActionType: action.OpenAccount, FulfillmentType: fulfillment.InitializeLockedTimelockAccount},
			{State: fulfillment.StatePending, ActionType: action.CloseDormantAccount, FulfillmentType: fulfillment.CloseDormantTimelockAccount},
			{State: fulfillment.StatePending, ActionType: action.CloseDormantAccount, FulfillmentType: fulfillment.CloseDormantTimelockAccount},
			{State: fulfillment.StatePending, ActionType: action.CloseDormantAccount, FulfillmentType: fulfillment.CloseDormantTimelockAccount},
			{State: fulfillment.StatePending, ActionType: action.CloseDormantAccount, FulfillmentType: fulfillment.CloseDormantTimelockAccount},

			// Not included in counts
			{State: fulfillment.StateUnknown, ActionType: action.CloseEmptyAccount, FulfillmentType: fulfillment.CloseEmptyTimelockAccount},
			{State: fulfillment.StateFailed, ActionType: action.CloseEmptyAccount, FulfillmentType: fulfillment.CloseEmptyTimelockAccount},
			{State: fulfillment.StateRevoked, ActionType: action.CloseEmptyAccount, FulfillmentType: fulfillment.CloseEmptyTimelockAccount},
			{State: fulfillment.StateConfirmed, ActionType: action.CloseEmptyAccount, FulfillmentType: fulfillment.CloseEmptyTimelockAccount},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range records {
			record.Intent = fmt.Sprintf("i%d", i+1)
			record.IntentType = intent.OpenAccounts
			record.Data = []byte(fmt.Sprintf("d%d", i+1))
			record.Signature = pointer.String(fmt.Sprintf("t%d", i+1))
			record.Nonce = pointer.String(fmt.Sprintf("n%d", i+1))
			record.Blockhash = pointer.String(fmt.Sprintf("bh%d", i+1))
			record.Source = fmt.Sprintf("s%d", i+1)
		}

		require.NoError(t, s.PutAll(ctx, records...))

		counts, err := s.CountPendingByType(ctx)
		require.NoError(t, err)

		assert.Len(t, counts, 2)
		assert.EqualValues(t, 2, counts[fulfillment.InitializeLockedTimelockAccount])
		assert.EqualValues(t, 4, counts[fulfillment.CloseDormantTimelockAccount])
	})
}

func testTreasuryQueries(t *testing.T, s fulfillment.Store) {
	t.Run("testTreasuryQueries", func(t *testing.T) {
		ctx := context.Background()

		records := []*fulfillment.Record{
			// Everything is a candidate for update
			{Source: "treasury1", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 1},
			{Source: "treasury1", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 2},
			{Source: "treasury1", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 3},
			{Source: "treasury1", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 4},
			{Source: "treasury1", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 5},

			// Everything is a candidate for update
			{Source: "treasury2", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 1},
			{Source: "treasury2", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 2},
			{Source: "treasury2", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 3},
			{Source: "treasury2", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 4},
			{Source: "treasury2", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 5},

			// Everything is a candidate for update
			{Source: "treasury3", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 1},
			{Source: "treasury3", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 2},
			{Source: "treasury3", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 3},
			{Source: "treasury3", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 4},
			{Source: "treasury3", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 5},

			// Multiple states
			{Source: "treasury4", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 1},
			{Source: "treasury4", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StatePending, DisableActiveScheduling: true, IntentOrderingIndex: 2},
			{Source: "treasury4", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateConfirmed, DisableActiveScheduling: true, IntentOrderingIndex: 3},
			{Source: "treasury4", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateFailed, DisableActiveScheduling: true, IntentOrderingIndex: 4},
			{Source: "treasury4", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateRevoked, DisableActiveScheduling: true, IntentOrderingIndex: 5},

			// Multiple fulfillment thypes
			{Source: "treasury5", FulfillmentType: fulfillment.TransferWithCommitment, State: fulfillment.StateUnknown, DisableActiveScheduling: true, IntentOrderingIndex: 1},
			{Source: "treasury5", FulfillmentType: fulfillment.SaveRecentRoot, State: fulfillment.StatePending, DisableActiveScheduling: true, IntentOrderingIndex: 2},
			{Source: "treasury5", FulfillmentType: fulfillment.InitializeLockedTimelockAccount, State: fulfillment.StateConfirmed, DisableActiveScheduling: true, IntentOrderingIndex: 3},
			{Source: "treasury5", FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority, State: fulfillment.StateFailed, DisableActiveScheduling: true, IntentOrderingIndex: 4},
			{Source: "treasury5", FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority, State: fulfillment.StateRevoked, DisableActiveScheduling: true, IntentOrderingIndex: 5},
		}

		// Fill in required fields that have no relevancy to this test
		for i, record := range records {
			record.Intent = fmt.Sprintf("i%d", i+1)
			record.IntentType = intent.SendPrivatePayment
			record.ActionType = action.PrivateTransfer
		}
		require.NoError(t, s.PutAll(ctx, records...))

		// Everything is updated
		updateCount, err := s.ActivelyScheduleTreasuryAdvances(ctx, "treasury1", 10, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 5, updateCount)
		for _, record := range records {
			actual, err := s.GetById(ctx, record.Id)
			require.NoError(t, err)
			assert.Equal(t, record.Source == "treasury1", !actual.DisableActiveScheduling)
		}

		// Everything updated, so should get an empty count without error
		updateCount, err = s.ActivelyScheduleTreasuryAdvances(ctx, "treasury1", 10, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 0, updateCount)

		// Subset matching intent ordering index is updated
		updateCount, err = s.ActivelyScheduleTreasuryAdvances(ctx, "treasury2", 3, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 2, updateCount)
		for _, record := range records {
			if record.Source != "treasury2" {
				continue
			}

			actual, err := s.GetById(ctx, record.Id)
			require.NoError(t, err)
			assert.Equal(t, record.IntentOrderingIndex < 3, !actual.DisableActiveScheduling)
		}

		// Subset limited by limit value is updated
		updateCount, err = s.ActivelyScheduleTreasuryAdvances(ctx, "treasury3", 10, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 2, updateCount)
		var totalUpdated int
		for _, record := range records {
			if record.Source != "treasury3" {
				continue
			}

			actual, err := s.GetById(ctx, record.Id)
			require.NoError(t, err)

			if !actual.DisableActiveScheduling {
				totalUpdated++
			}
		}
		assert.Equal(t, 2, totalUpdated)

		// Subset matching expected state is updated
		updateCount, err = s.ActivelyScheduleTreasuryAdvances(ctx, "treasury4", 10, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 1, updateCount)
		for _, record := range records {
			if record.Source != "treasury4" {
				continue
			}

			actual, err := s.GetById(ctx, record.Id)
			require.NoError(t, err)
			assert.Equal(t, record.State == fulfillment.StateUnknown, !actual.DisableActiveScheduling)
		}

		// Subset matching expected fulfillment type is updated
		updateCount, err = s.ActivelyScheduleTreasuryAdvances(ctx, "treasury5", 10, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 1, updateCount)
		for _, record := range records {
			if record.Source != "treasury5" {
				continue
			}

			actual, err := s.GetById(ctx, record.Id)
			require.NoError(t, err)
			assert.Equal(t, record.FulfillmentType == fulfillment.TransferWithCommitment, !actual.DisableActiveScheduling)
		}
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *fulfillment.Record) {
	assert.Equal(t, obj1.Intent, obj2.Intent)
	assert.Equal(t, obj1.IntentType, obj2.IntentType)
	assert.Equal(t, obj1.ActionId, obj2.ActionId)
	assert.Equal(t, obj1.ActionType, obj2.ActionType)
	assert.Equal(t, obj1.FulfillmentType, obj2.FulfillmentType)
	assert.EqualValues(t, obj1.Data, obj2.Data)
	assert.EqualValues(t, obj1.Signature, obj2.Signature)
	assert.EqualValues(t, obj1.Nonce, obj2.Nonce)
	assert.EqualValues(t, obj1.Blockhash, obj2.Blockhash)
	assert.Equal(t, obj1.Source, obj2.Source)
	assert.EqualValues(t, obj1.Destination, obj2.Destination)
	assert.Equal(t, obj1.IntentOrderingIndex, obj2.IntentOrderingIndex)
	assert.Equal(t, obj1.ActionOrderingIndex, obj2.ActionOrderingIndex)
	assert.Equal(t, obj1.FulfillmentOrderingIndex, obj2.FulfillmentOrderingIndex)
	assert.Equal(t, obj1.DisableActiveScheduling, obj2.DisableActiveScheduling)
	assert.EqualValues(t, obj1.InitiatorPhoneNumber, obj2.InitiatorPhoneNumber)
	assert.Equal(t, obj1.State, obj2.State)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}
