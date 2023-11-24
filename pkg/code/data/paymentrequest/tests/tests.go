package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
)

func RunTests(t *testing.T, s paymentrequest.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s paymentrequest.Store){
		testRoundTrip,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s paymentrequest.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent")
		require.Error(t, err)
		assert.Equal(t, paymentrequest.ErrPaymentRequestNotFound, err)
		assert.Nil(t, actual)

		expected := &paymentrequest.Record{
			Intent:                  "test_intent",
			DestinationTokenAccount: "destination",
			ExchangeCurrency:        "usd",
			NativeAmount:            2.46,
			ExchangeRate:            pointer.Float64(1.23),
			Quantity:                pointer.Uint64(kin.ToQuarks(2)),
			Domain:                  pointer.String("example.com"),
			IsVerified:              true,
			CreatedAt:               time.Now(),
		}
		cloned := expected.Clone()
		err = s.Put(ctx, expected)
		require.NoError(t, err)

		assert.Equal(t, paymentrequest.ErrPaymentRequestAlreadyExists, s.Put(ctx, expected))
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *paymentrequest.Record) {
	assert.Equal(t, obj1.Intent, obj2.Intent)
	assert.Equal(t, obj1.DestinationTokenAccount, obj2.DestinationTokenAccount)
	assert.Equal(t, obj1.ExchangeCurrency, obj2.ExchangeCurrency)
	assert.Equal(t, obj1.NativeAmount, obj2.NativeAmount)
	assert.EqualValues(t, obj1.ExchangeRate, obj2.ExchangeRate)
	assert.EqualValues(t, obj1.Quantity, obj2.Quantity)
	assert.EqualValues(t, obj1.Domain, obj2.Domain)
	assert.Equal(t, obj1.IsVerified, obj2.IsVerified)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}
