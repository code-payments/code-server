package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

func RunTests(t *testing.T, s deposit.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s deposit.Store){
		testRoundTrip,
		testGetAmounts,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s deposit.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()

		record := &deposit.Record{
			Signature:         "txn",
			Destination:       "destination",
			Amount:            1,
			UsdMarketValue:    1.23,
			Slot:              0,
			ConfirmationState: transaction.ConfirmationConfirmed,
		}
		cloned := record.Clone()

		_, err := s.Get(ctx, record.Signature, record.Destination)
		assert.Equal(t, deposit.ErrDepositNotFound, err)

		require.NoError(t, s.Save(ctx, record))
		assert.True(t, record.Id > 0)
		assert.True(t, record.CreatedAt.After(start))

		actual, err := s.Get(ctx, cloned.Signature, cloned.Destination)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		record.Slot = 12345
		record.ConfirmationState = transaction.ConfirmationFinalized
		cloned = record.Clone()

		require.NoError(t, s.Save(ctx, record))

		actual, err = s.Get(ctx, cloned.Signature, cloned.Destination)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

	})
}

func testGetAmounts(t *testing.T, s deposit.Store) {
	t.Run("testGetAmounts", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		destination1 := "destination1"
		destination2 := "destination2"

		quarks, err := s.GetKinAmount(ctx, destination1)
		require.NoError(t, err)
		assert.EqualValues(t, 0, quarks)

		quarksByAccount, err := s.GetKinAmountBatch(ctx, destination1, destination2)
		require.NoError(t, err)
		require.Len(t, quarksByAccount, 2)
		assert.EqualValues(t, 0, quarksByAccount[destination1])
		assert.EqualValues(t, 0, quarksByAccount[destination2])

		usd, err := s.GetUsdAmount(ctx, destination1)
		require.NoError(t, err)
		assert.EqualValues(t, 0, usd)

		records := []*deposit.Record{
			{Signature: "txn1", Destination: destination1, Amount: 1, UsdMarketValue: 2, Slot: 12345, ConfirmationState: transaction.ConfirmationConfirmed},
			{Signature: "txn2", Destination: destination1, Amount: 10, UsdMarketValue: 20, Slot: 12345, ConfirmationState: transaction.ConfirmationFailed},
			{Signature: "txn3", Destination: destination1, Amount: 100, UsdMarketValue: 200, Slot: 12345, ConfirmationState: transaction.ConfirmationFinalized},
			{Signature: "txn4", Destination: destination1, Amount: 1000, UsdMarketValue: 2000, Slot: 12345, ConfirmationState: transaction.ConfirmationFinalized},
			{Signature: "txn1", Destination: destination2, Amount: 10000, UsdMarketValue: 20000, Slot: 12345, ConfirmationState: transaction.ConfirmationFinalized},
		}
		for _, record := range records {
			require.NoError(t, s.Save(ctx, record))

			assert.True(t, record.Id > 0)
			assert.True(t, record.CreatedAt.After(start))
		}

		quarks, err = s.GetKinAmount(ctx, destination1)
		require.NoError(t, err)
		assert.EqualValues(t, 1100, quarks)

		quarks, err = s.GetKinAmount(ctx, destination2)
		require.NoError(t, err)
		assert.EqualValues(t, 10000, quarks)

		quarksByAccount, err = s.GetKinAmountBatch(ctx, destination1, destination2, destination2)
		require.NoError(t, err)
		require.Len(t, quarksByAccount, 2)
		assert.EqualValues(t, 1100, quarksByAccount[destination1])
		assert.EqualValues(t, 10000, quarksByAccount[destination2])

		usd, err = s.GetUsdAmount(ctx, destination1)
		require.NoError(t, err)
		assert.EqualValues(t, 2200, usd)

		usd, err = s.GetUsdAmount(ctx, destination2)
		require.NoError(t, err)
		assert.EqualValues(t, 20000, usd)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *deposit.Record) {
	assert.Equal(t, obj1.Signature, obj2.Signature)
	assert.Equal(t, obj1.Destination, obj2.Destination)
	assert.Equal(t, obj1.Amount, obj2.Amount)
	assert.Equal(t, obj1.UsdMarketValue, obj2.UsdMarketValue)
	assert.Equal(t, obj1.Slot, obj2.Slot)
	assert.Equal(t, obj1.ConfirmationState, obj2.ConfirmationState)
}
