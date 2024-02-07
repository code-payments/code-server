package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s paymentrequest.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s paymentrequest.Store){
		testRoundTrip,
		testInvalidRecord,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s paymentrequest.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		for i, fees := range [][]*paymentrequest.Fee{
			nil,
			{
				{
					DestinationTokenAccount: "destination2",
				},
				{
					DestinationTokenAccount: "destination3",
				},
			},
		} {
			ctx := context.Background()

			intentId := fmt.Sprintf("test_intent%d", i)

			actual, err := s.Get(ctx, intentId)
			require.Error(t, err)
			assert.Equal(t, paymentrequest.ErrPaymentRequestNotFound, err)
			assert.Nil(t, actual)

			expected := &paymentrequest.Record{
				Intent:                  intentId,
				DestinationTokenAccount: pointer.String("destination1"),
				ExchangeCurrency:        pointer.String("usd"),
				NativeAmount:            pointer.Float64(2.46),
				ExchangeRate:            pointer.Float64(1.23),
				Quantity:                pointer.Uint64(kin.ToQuarks(2)),
				Fees:                    fees,
				Domain:                  pointer.String("example.com"),
				IsVerified:              true,
				CreatedAt:               time.Now(),
			}
			cloned := expected.Clone()
			err = s.Put(ctx, expected)
			require.NoError(t, err)

			assert.Equal(t, paymentrequest.ErrPaymentRequestAlreadyExists, s.Put(ctx, expected))
			require.NoError(t, err)

			actual, err = s.Get(ctx, intentId)
			require.NoError(t, err)
			assertEquivalentRecords(t, &cloned, actual)
			assert.True(t, actual.Id > 0)
		}
	})
}

func testInvalidRecord(t *testing.T, s paymentrequest.Store) {
	t.Run("testInvalidRecord", func(t *testing.T) {
		ctx := context.Background()

		expected := &paymentrequest.Record{
			Intent:                  "test_intent",
			DestinationTokenAccount: pointer.String("destination1"),
			ExchangeCurrency:        pointer.String("usd"),
			NativeAmount:            pointer.Float64(2.46),
			ExchangeRate:            pointer.Float64(1.23),
			Quantity:                pointer.Uint64(kin.ToQuarks(2)),
			Fees: []*paymentrequest.Fee{
				{
					DestinationTokenAccount: "destination2",
				},
				{
					DestinationTokenAccount: "destination2",
				},
			},
			Domain:     pointer.String("example.com"),
			IsVerified: true,
			CreatedAt:  time.Now(),
		}

		assert.Equal(t, paymentrequest.ErrInvalidPaymentRequest, s.Put(ctx, expected))

		_, err := s.Get(ctx, "test_intent")
		assert.Equal(t, paymentrequest.ErrPaymentRequestNotFound, err)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *paymentrequest.Record) {
	assert.Equal(t, obj1.Intent, obj2.Intent)
	assert.EqualValues(t, obj1.DestinationTokenAccount, obj2.DestinationTokenAccount)
	assert.EqualValues(t, obj1.ExchangeCurrency, obj2.ExchangeCurrency)
	assert.EqualValues(t, obj1.NativeAmount, obj2.NativeAmount)
	assert.EqualValues(t, obj1.ExchangeRate, obj2.ExchangeRate)
	assert.EqualValues(t, obj1.Quantity, obj2.Quantity)
	assert.EqualValues(t, obj1.Domain, obj2.Domain)
	assert.Equal(t, obj1.IsVerified, obj2.IsVerified)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())

	require.Equal(t, len(obj1.Fees), len(obj2.Fees))
	for i := 0; i < len(obj1.Fees); i++ {
		assert.Equal(t, obj1.Fees[i].DestinationTokenAccount, obj2.Fees[i].DestinationTokenAccount)
	}
}
