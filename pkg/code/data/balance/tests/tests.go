package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/balance"
)

func RunTests(t *testing.T, s balance.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s balance.Store){
		testExternalCheckpointHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testExternalCheckpointHappyPath(t *testing.T, s balance.Store) {
	t.Run("testExternalCheckpointHappyPath", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetExternalCheckpoint(ctx, "token_account")
		assert.Equal(t, balance.ErrCheckpointNotFound, err)

		start := time.Now()

		expected := &balance.ExternalCheckpointRecord{
			TokenAccount:   "token_account",
			Quarks:         0,
			SlotCheckpoint: 0,
		}
		cloned := expected.Clone()

		require.NoError(t, s.SaveExternalCheckpoint(ctx, expected))
		assert.EqualValues(t, 1, expected.Id)
		assert.True(t, expected.LastUpdatedAt.After(start))

		actual, err := s.GetExternalCheckpoint(ctx, "token_account")
		require.NoError(t, err)
		assertEquivalentExternalCheckpoingRecords(t, actual, &cloned)

		start = time.Now()

		expected.Quarks = 12345
		expected.SlotCheckpoint = 10
		cloned = expected.Clone()

		require.NoError(t, s.SaveExternalCheckpoint(ctx, expected))
		assert.EqualValues(t, 1, expected.Id)
		assert.True(t, expected.LastUpdatedAt.After(start))

		actual, err = s.GetExternalCheckpoint(ctx, "token_account")
		require.NoError(t, err)
		assertEquivalentExternalCheckpoingRecords(t, actual, &cloned)

		expected.Quarks = 67890
		assert.Equal(t, balance.ErrStaleCheckpoint, s.SaveExternalCheckpoint(ctx, expected))

		actual, err = s.GetExternalCheckpoint(ctx, "token_account")
		require.NoError(t, err)
		assertEquivalentExternalCheckpoingRecords(t, actual, &cloned)

		expected.SlotCheckpoint -= 1
		assert.Equal(t, balance.ErrStaleCheckpoint, s.SaveExternalCheckpoint(ctx, expected))

		actual, err = s.GetExternalCheckpoint(ctx, "token_account")
		require.NoError(t, err)
		assertEquivalentExternalCheckpoingRecords(t, actual, &cloned)
	})
}

func assertEquivalentExternalCheckpoingRecords(t *testing.T, obj1, obj2 *balance.ExternalCheckpointRecord) {
	assert.Equal(t, obj1.TokenAccount, obj2.TokenAccount)
	assert.Equal(t, obj1.Quarks, obj2.Quarks)
	assert.Equal(t, obj1.SlotCheckpoint, obj2.SlotCheckpoint)
}
