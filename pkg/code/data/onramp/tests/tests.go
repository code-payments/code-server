package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/onramp"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

func RunTests(t *testing.T, s onramp.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s onramp.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s onramp.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		nonce := uuid.New()

		_, err := s.Get(ctx, nonce)
		assert.Equal(t, onramp.ErrPurchaseNotFound, err)

		start := time.Now()

		expected := &onramp.Record{
			Owner:    "owner",
			Platform: int(client.DeviceTypeIOS),
			Currency: "cad",
			Amount:   50.01,
			Nonce:    nonce,
		}
		cloned := expected.Clone()

		require.NoError(t, s.Put(ctx, expected))
		assert.EqualValues(t, 1, expected.Id)
		assert.True(t, expected.CreatedAt.After(start))

		actual, err := s.Get(ctx, nonce)
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)

		assert.Equal(t, onramp.ErrPurchaseAlreadyExists, s.Put(ctx, expected))

		actual, err = s.Get(ctx, nonce)
		require.NoError(t, err)
		assertEquivalentRecords(t, actual, &cloned)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *onramp.Record) {
	assert.Equal(t, obj1.Owner, obj2.Owner)
	assert.Equal(t, obj1.Platform, obj2.Platform)
	assert.Equal(t, obj1.Currency, obj2.Currency)
	assert.Equal(t, obj1.Amount, obj2.Amount)
	assert.Equal(t, obj1.Nonce, obj2.Nonce)
}
