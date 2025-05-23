package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

func RunTests(t *testing.T, s timelock.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s timelock.Store){
		testHappyPath,
		testBatchedMethods,
		testGetAllByState,
		testGetCountByState,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s timelock.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		start := time.Now()

		ctx := context.Background()

		expected := &timelock.Record{
			Address: "state",
			Bump:    254,

			VaultAddress: "vault",
			VaultBump:    255,
			VaultOwner:   "owner",
			VaultState:   timelock_token.StateUnknown,

			DepositPdaAddress: "deposit",
			DepositPdaBump:    253,

			Block: 123456,
		}
		cloned := expected.Clone()

		// Validate the record initially doesn't exist

		_, err := s.GetByAddress(ctx, expected.VaultOwner)
		assert.Equal(t, timelock.ErrTimelockNotFound, err)

		_, err = s.GetByVault(ctx, expected.VaultAddress)
		assert.Equal(t, timelock.ErrTimelockNotFound, err)

		_, err = s.GetByDepositPda(ctx, expected.DepositPdaAddress)
		assert.Equal(t, timelock.ErrTimelockNotFound, err)

		// Save the record

		require.NoError(t, s.Save(ctx, expected))
		assert.True(t, expected.Id > 0)
		assert.True(t, expected.LastUpdatedAt.After(start))

		// Ensure we can fetch the same record by all supported indices

		actual, err := s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)

		actual, err = s.GetByVault(ctx, expected.VaultAddress)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)

		actual, err = s.GetByDepositPda(ctx, expected.DepositPdaAddress)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)

		initialBlock := expected.Block

		// Update the record's state

		previousLastUpdatedTs := expected.LastUpdatedAt

		unlockedAt := uint64(time.Now().Unix())
		expected.UnlockAt = &unlockedAt

		expected.VaultState = timelock_token.StateUnlocked

		// Try to save the record with old blockchain data, which should fail

		expected.Block = initialBlock - 1
		time.Sleep(time.Millisecond)
		err = s.Save(ctx, expected)
		assert.Equal(t, timelock.ErrStaleTimelockState, err)
		assert.Equal(t, previousLastUpdatedTs, expected.LastUpdatedAt)

		// Save the record with new blockchain data

		expected.Block = initialBlock + 1
		cloned = expected.Clone()
		time.Sleep(time.Millisecond)
		require.NoError(t, s.Save(ctx, expected))
		assert.True(t, expected.LastUpdatedAt.After(previousLastUpdatedTs))

		// Ensure we can fetch the updated record by all supported indices

		actual, err = s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)

		actual, err = s.GetByVault(ctx, expected.VaultAddress)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)

		actual, err = s.GetByDepositPda(ctx, expected.DepositPdaAddress)
		require.NoError(t, err)
		assertEquivalentRecords(t, cloned, actual)
	})
}

func testBatchedMethods(t *testing.T, s timelock.Store) {
	t.Run("testBatchedMethods", func(t *testing.T) {
		ctx := context.Background()

		var records []*timelock.Record
		for i := 0; i < 100; i++ {
			record := &timelock.Record{
				Address: fmt.Sprintf("state%d", i),
				Bump:    254,

				VaultAddress: fmt.Sprintf("vault%d", i),
				VaultBump:    255,
				VaultOwner:   fmt.Sprintf("owner%d", i),
				VaultState:   timelock_token.StateUnknown,

				DepositPdaAddress: fmt.Sprintf("deposit%d", i),
				DepositPdaBump:    253,

				Block: uint64(i),
			}

			require.NoError(t, s.Save(ctx, record))

			records = append(records, record.Clone())
		}

		actual, err := s.GetByVaultBatch(ctx, "vault0", "vault1")
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[0], actual[records[0].VaultAddress])
		assertEquivalentRecords(t, records[1], actual[records[1].VaultAddress])

		actual, err = s.GetByVaultBatch(ctx, "vault0", "vault1", "vault2", "vault3", "vault4")
		require.NoError(t, err)
		require.Len(t, actual, 5)
		assertEquivalentRecords(t, records[0], actual[records[0].VaultAddress])
		assertEquivalentRecords(t, records[1], actual[records[1].VaultAddress])
		assertEquivalentRecords(t, records[2], actual[records[2].VaultAddress])
		assertEquivalentRecords(t, records[3], actual[records[3].VaultAddress])
		assertEquivalentRecords(t, records[4], actual[records[4].VaultAddress])

		_, err = s.GetByVaultBatch(ctx, "not-found")
		assert.Equal(t, timelock.ErrTimelockNotFound, err)

		_, err = s.GetByVaultBatch(ctx, "vault0", "not-found")
		assert.Equal(t, timelock.ErrTimelockNotFound, err)
	})
}

func testGetAllByState(t *testing.T, s timelock.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		var expected []*timelock.Record
		for i := 0; i < 100; i++ {
			record := &timelock.Record{
				Address: fmt.Sprintf("state%d", i),
				Bump:    254,

				VaultAddress: fmt.Sprintf("vault%d", i),
				VaultBump:    255,
				VaultOwner:   fmt.Sprintf("owner%d", i),
				VaultState:   timelock_token.StateUnknown,

				DepositPdaAddress: fmt.Sprintf("deposit%d", i),
				DepositPdaBump:    253,

				Block: uint64(i),
			}

			require.NoError(t, s.Save(ctx, record))

			expected = append(expected, record.Clone())
		}

		_, err := s.GetAllByState(ctx, timelock_token.StateLocked, query.EmptyCursor, 10, query.Ascending)
		assert.Equal(t, timelock.ErrTimelockNotFound, err)

		var cursor query.Cursor
		var actual []*timelock.Record
		for {
			records, err := s.GetAllByState(ctx, timelock_token.StateUnknown, cursor, 10, query.Ascending)
			if err == timelock.ErrTimelockNotFound {
				break
			}
			assert.Len(t, records, 10)

			actual = append(actual, records...)
			cursor = query.ToCursor(records[len(records)-1].Id)
		}

		require.Len(t, actual, 100)
		for i, record := range expected {
			assertEquivalentRecords(t, record, actual[i])
		}

		cursor = query.EmptyCursor
		actual = nil
		for {
			records, err := s.GetAllByState(ctx, timelock_token.StateUnknown, cursor, 10, query.Descending)
			if err == timelock.ErrTimelockNotFound {
				break
			}
			assert.Len(t, records, 10)

			actual = append(actual, records...)
			cursor = query.ToCursor(records[len(records)-1].Id)
		}

		require.Len(t, actual, 100)
		for i, record := range expected {
			assertEquivalentRecords(t, record, actual[len(actual)-i-1])
		}
	})
}

func testGetCountByState(t *testing.T, s timelock.Store) {
	t.Run("testGetCountByState", func(t *testing.T) {
		ctx := context.Background()

		for _, state := range []timelock_token.TimelockState{
			timelock_token.StateUnknown,
			timelock_token.StateLocked,
			timelock_token.StateWaitingForTimeout,
			timelock_token.StateUnlocked,
		} {
			for i := 0; i < int(state); i++ {
				record := &timelock.Record{
					Address: fmt.Sprintf("state-%s-%d", state, i),
					Bump:    254,

					VaultAddress: fmt.Sprintf("vault-%s-%d", state, i),
					VaultBump:    255,
					VaultOwner:   fmt.Sprintf("owner-%s-%d", state, i),
					VaultState:   state,

					DepositPdaAddress: fmt.Sprintf("deposit-%s-%d", state, i),
					DepositPdaBump:    253,
				}

				require.NoError(t, s.Save(ctx, record))
			}

			count, err := s.GetCountByState(ctx, state)
			require.NoError(t, err)
			assert.EqualValues(t, state, count)
		}
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *timelock.Record) {
	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.Bump, obj2.Bump)

	assert.Equal(t, obj1.VaultAddress, obj2.VaultAddress)
	assert.Equal(t, obj1.VaultBump, obj2.VaultBump)
	assert.Equal(t, obj1.VaultOwner, obj2.VaultOwner)
	assert.Equal(t, obj1.VaultState, obj2.VaultState)

	assert.EqualValues(t, obj1.UnlockAt, obj2.UnlockAt)

	assert.Equal(t, obj1.DepositPdaAddress, obj2.DepositPdaAddress)
	assert.Equal(t, obj1.DepositPdaBump, obj2.DepositPdaBump)

	assert.Equal(t, obj1.Block, obj2.Block)
}
