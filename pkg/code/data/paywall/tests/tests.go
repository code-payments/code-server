package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/paywall"
)

func RunTests(t *testing.T, s paywall.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s paywall.Store){
		testRoundTrip,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s paywall.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.GetByShortPath(ctx, "abcd1234")
		require.Error(t, err)
		assert.Equal(t, paywall.ErrPaywallNotFound, err)
		assert.Nil(t, actual)

		expected := &paywall.Record{
			OwnerAccount:            "owner",
			DestinationTokenAccount: "destination",
			ExchangeCurrency:        "usd",
			NativeAmount:            0.25,
			RedirectUrl:             "http://redirect.to/me",
			ShortPath:               "abcd1234",
			Signature:               "signature",
			CreatedAt:               time.Now(),
		}
		cloned := expected.Clone()
		err = s.Put(ctx, expected)
		require.NoError(t, err)

		assert.Equal(t, paywall.ErrPaywallExists, s.Put(ctx, expected))
		require.NoError(t, err)

		actual, err = s.GetByShortPath(ctx, "abcd1234")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *paywall.Record) {
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.Equal(t, obj1.DestinationTokenAccount, obj2.DestinationTokenAccount)
	assert.Equal(t, obj1.ExchangeCurrency, obj2.ExchangeCurrency)
	assert.Equal(t, obj1.NativeAmount, obj2.NativeAmount)
	assert.Equal(t, obj1.RedirectUrl, obj2.RedirectUrl)
	assert.Equal(t, obj1.ShortPath, obj2.ShortPath)
	assert.Equal(t, obj1.Signature, obj2.Signature)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}
