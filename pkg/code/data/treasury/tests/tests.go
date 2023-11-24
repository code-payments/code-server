package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/database/query"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
)

func RunTests(t *testing.T, s treasury.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s treasury.Store){
		testTreasuryPoolHappyPath,
		testGetAllByState,
		testFundingHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testTreasuryPoolHappyPath(t *testing.T, s treasury.Store) {
	t.Run("testTreasuryPoolHappyPath", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()

		expected := &treasury.Record{
			DataVersion: splitter_token.DataVersion1,

			Name: "name",

			Address: "treasury",
			Bump:    255,

			Vault:     "vault",
			VaultBump: 254,

			Authority: "code",

			MerkleTreeLevels: 32,

			CurrentIndex:    1,
			HistoryListSize: 3,
			HistoryList:     []string{"root1", "root2", "root3"},

			SolanaBlock: 3,

			State: treasury.TreasuryPoolStateAvailable,
		}
		cloned := expected.Clone()

		_, err := s.GetByName(ctx, expected.Name)
		assert.Equal(t, treasury.ErrTreasuryPoolNotFound, err)

		_, err = s.GetByAddress(ctx, expected.Address)
		assert.Equal(t, treasury.ErrTreasuryPoolNotFound, err)

		_, err = s.GetByVault(ctx, expected.Vault)
		assert.Equal(t, treasury.ErrTreasuryPoolNotFound, err)

		require.NoError(t, s.Save(ctx, expected))
		assert.True(t, expected.Id > 0)
		assert.True(t, expected.LastUpdatedAt.After(start))
		assertEquivalentTreasuryPoolRecords(t, cloned, expected)

		actual, err := s.GetByName(ctx, expected.Name)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		actual, err = s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		actual, err = s.GetByVault(ctx, expected.Vault)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		for _, blockCount := range []uint64{cloned.SolanaBlock - 1, cloned.SolanaBlock} {
			expected.CurrentIndex = 2
			expected.SolanaBlock = uint64(blockCount)
			expected.HistoryList = []string{"root4", "root5", "root6"}
			err = s.Save(ctx, expected)
			assert.Equal(t, treasury.ErrStaleTreasuryPoolState, err)

			actual, err = s.GetByName(ctx, expected.Name)
			require.NoError(t, err)
			assertEquivalentTreasuryPoolRecords(t, cloned, actual)

			actual, err = s.GetByAddress(ctx, expected.Address)
			require.NoError(t, err)
			assertEquivalentTreasuryPoolRecords(t, cloned, actual)

			actual, err = s.GetByVault(ctx, expected.Vault)
			require.NoError(t, err)
			assertEquivalentTreasuryPoolRecords(t, cloned, actual)
		}

		expected.CurrentIndex = 2
		expected.SolanaBlock += 1
		cloned = expected.Clone()
		require.NoError(t, s.Save(ctx, expected))
		assertEquivalentTreasuryPoolRecords(t, cloned, expected)

		actual, err = s.GetByName(ctx, expected.Name)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		actual, err = s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		actual, err = s.GetByVault(ctx, expected.Vault)
		require.NoError(t, err)
		assertEquivalentTreasuryPoolRecords(t, cloned, actual)

		records, err := s.GetAllByState(ctx, cloned.State, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		require.Len(t, records, 1)
		assertEquivalentTreasuryPoolRecords(t, cloned, records[0])
	})
}

func testGetAllByState(t *testing.T, s treasury.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 10, query.Ascending)
		assert.Equal(t, treasury.ErrTreasuryPoolNotFound, err)

		expected := []*treasury.Record{
			{DataVersion: splitter_token.DataVersion1, Name: "name1", Address: "treasury1", Vault: "vault1", Authority: "code", MerkleTreeLevels: 32, CurrentIndex: 0, HistoryListSize: 1, HistoryList: []string{"root1"}, SolanaBlock: 1, State: treasury.TreasuryPoolStateAvailable},
			{DataVersion: splitter_token.DataVersion1, Name: "name2", Address: "treasury2", Vault: "vault2", Authority: "code", MerkleTreeLevels: 32, CurrentIndex: 0, HistoryListSize: 1, HistoryList: []string{"root2"}, SolanaBlock: 2, State: treasury.TreasuryPoolStateAvailable},
			{DataVersion: splitter_token.DataVersion1, Name: "name3", Address: "treasury3", Vault: "vault3", Authority: "code", MerkleTreeLevels: 32, CurrentIndex: 0, HistoryListSize: 1, HistoryList: []string{"root3"}, SolanaBlock: 3, State: treasury.TreasuryPoolStateAvailable},
			{DataVersion: splitter_token.DataVersion1, Name: "name4", Address: "treasury4", Vault: "vault4", Authority: "code", MerkleTreeLevels: 32, CurrentIndex: 0, HistoryListSize: 1, HistoryList: []string{"root4"}, SolanaBlock: 4, State: treasury.TreasuryPoolStateDeprecated},
			{DataVersion: splitter_token.DataVersion1, Name: "name5", Address: "treasury5", Vault: "vault5", Authority: "code", MerkleTreeLevels: 32, CurrentIndex: 0, HistoryListSize: 1, HistoryList: []string{"root5"}, SolanaBlock: 5, State: treasury.TreasuryPoolStateDeprecated},
		}
		for _, record := range expected {
			require.NoError(t, s.Save(ctx, record))
		}

		_, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateUnknown, query.EmptyCursor, 10, query.Ascending)
		assert.Equal(t, treasury.ErrTreasuryPoolNotFound, err)

		actual, err := s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		assert.Len(t, actual, 3)

		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateDeprecated, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		assert.Len(t, actual, 2)

		// Check items (asc)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assert.Equal(t, "treasury1", actual[0].Address)
		assert.Equal(t, "treasury2", actual[1].Address)
		assert.Equal(t, "treasury3", actual[2].Address)

		// Check items (desc)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assert.Equal(t, "treasury3", actual[0].Address)
		assert.Equal(t, "treasury2", actual[1].Address)
		assert.Equal(t, "treasury1", actual[2].Address)

		// Check items (asc + limit)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 2, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "treasury1", actual[0].Address)
		assert.Equal(t, "treasury2", actual[1].Address)

		// Check items (desc + limit)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.EmptyCursor, 2, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "treasury3", actual[0].Address)
		assert.Equal(t, "treasury2", actual[1].Address)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.ToCursor(1), 5, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "treasury2", actual[0].Address)
		assert.Equal(t, "treasury3", actual[1].Address)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, treasury.TreasuryPoolStateAvailable, query.ToCursor(3), 5, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "treasury2", actual[0].Address)
		assert.Equal(t, "treasury1", actual[1].Address)
	})
}

func testFundingHappyPath(t *testing.T, s treasury.Store) {
	t.Run("testFundingHappyPath", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.GetTotalAvailableFunds(ctx, "vault1")
		require.NoError(t, err)
		assert.EqualValues(t, 0, actual)

		records := []*treasury.FundingHistoryRecord{
			{Vault: "vault1", DeltaQuarks: 1, TransactionId: "txn1", State: treasury.FundingStateUnknown, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: 10, TransactionId: "txn2", State: treasury.FundingStatePending, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: 100, TransactionId: "txn3", State: treasury.FundingStateFailed, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: 1000, TransactionId: "txn4", State: treasury.FundingStateConfirmed, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: -1, TransactionId: "txn5", State: treasury.FundingStateUnknown, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: -10, TransactionId: "txn6", State: treasury.FundingStatePending, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: -100, TransactionId: "txn7", State: treasury.FundingStateConfirmed, CreatedAt: time.Now()},
			{Vault: "vault1", DeltaQuarks: -1000, TransactionId: "txn8", State: treasury.FundingStateFailed, CreatedAt: time.Now()},

			{Vault: "vault2", DeltaQuarks: -1, TransactionId: "txn9", State: treasury.FundingStateConfirmed, CreatedAt: time.Now()},

			{Vault: "vault3", DeltaQuarks: 1000, TransactionId: "txn10", State: treasury.FundingStateConfirmed, CreatedAt: time.Now()},
		}

		for _, record := range records {
			require.NoError(t, s.SaveFunding(ctx, record))
		}

		actual, err = s.GetTotalAvailableFunds(ctx, "vault1")
		require.NoError(t, err)
		assert.EqualValues(t, 889, actual)

		_, err = s.GetTotalAvailableFunds(ctx, "vault2")
		assert.Equal(t, treasury.ErrNegativeFunding, err)

		actual, err = s.GetTotalAvailableFunds(ctx, "vault3")
		require.NoError(t, err)
		assert.EqualValues(t, 1000, actual)
	})
}

func assertEquivalentTreasuryPoolRecords(t *testing.T, obj1, obj2 *treasury.Record) {
	assert.Equal(t, obj1.DataVersion, obj2.DataVersion)

	assert.Equal(t, obj1.Name, obj2.Name)

	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.Bump, obj2.Bump)

	assert.Equal(t, obj1.Vault, obj2.Vault)
	assert.Equal(t, obj1.VaultBump, obj2.VaultBump)

	assert.Equal(t, obj1.Authority, obj2.Authority)

	assert.Equal(t, obj1.MerkleTreeLevels, obj2.MerkleTreeLevels)

	assert.Equal(t, obj1.CurrentIndex, obj2.CurrentIndex)
	assert.Equal(t, obj1.HistoryListSize, obj2.HistoryListSize)
	assert.EqualValues(t, obj1.HistoryList, obj2.HistoryList)

	assert.Equal(t, obj1.SolanaBlock, obj2.SolanaBlock)

	assert.Equal(t, obj1.State, obj2.State)
}
