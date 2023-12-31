package tests

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/database/query"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

func RunTests(t *testing.T, s commitment.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s commitment.Store){
		testRoundTrip,
		testUpdateConstraints,
		testGetAllByState,
		testGetUpgradeableByOwner,
		testGetTreasuryPoolDeficit,
		testCounts,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s commitment.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		expected := &commitment.Record{
			DataVersion: splitter_token.DataVersion1,

			Address: "address",
			Bump:    255,

			Pool:       "pool",
			PoolBump:   254,
			RecentRoot: "root",
			Transcript: "transcript",

			Destination: "destination",
			Amount:      12345,

			Vault:     "vault",
			VaultBump: 253,

			Intent:   "intent",
			ActionId: 1,

			Owner: "owner",

			State: commitment.StateOpen,

			CreatedAt: time.Now(),
		}

		_, err := s.GetByAddress(ctx, expected.Address)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		_, err = s.GetByVault(ctx, expected.Vault)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		_, err = s.GetByAction(ctx, expected.Intent, expected.ActionId)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		cloned := expected.Clone()
		require.NoError(t, s.Save(ctx, cloned))
		assert.EqualValues(t, 1, cloned.Id)

		actual, err := s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)

		actual, err = s.GetByVault(ctx, expected.Vault)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)

		actual, err = s.GetByAction(ctx, expected.Intent, expected.ActionId)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)

		otherCommitmentVault := "other-vault"
		expected.TreasuryRepaid = true
		expected.RepaymentDivertedTo = &otherCommitmentVault
		expected.State = commitment.StateClosed
		cloned = expected.Clone()
		require.NoError(t, s.Save(ctx, cloned))

		actual, err = s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)

		actual, err = s.GetByVault(ctx, expected.Vault)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)

		actual, err = s.GetByAction(ctx, expected.Intent, expected.ActionId)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)
	})
}

func testUpdateConstraints(t *testing.T, s commitment.Store) {
	t.Run("testUpdateConstraints", func(t *testing.T) {
		ctx := context.Background()

		otherCommitmentVault1 := "other-vault-1"
		otherCommitmentVault2 := "other-vault-2"
		expected := &commitment.Record{
			DataVersion: splitter_token.DataVersion1,

			Address: "address",
			Bump:    255,

			Pool:       "pool",
			PoolBump:   254,
			RecentRoot: "root",
			Transcript: "transcript",

			Destination: "destination",
			Amount:      12345,

			Vault:     "vault",
			VaultBump: 253,

			Intent:   "intent",
			ActionId: 1,

			Owner: "owner",

			TreasuryRepaid:      true,
			RepaymentDivertedTo: &otherCommitmentVault1,

			State: commitment.StateClosed,

			CreatedAt: time.Now(),
		}
		require.NoError(t, s.Save(ctx, expected))

		cloned := expected.Clone()
		cloned.TreasuryRepaid = false
		assert.Equal(t, commitment.ErrInvalidCommitment, s.Save(ctx, cloned))

		cloned = expected.Clone()
		cloned.State = commitment.StateOpen
		assert.Equal(t, commitment.ErrInvalidCommitment, s.Save(ctx, cloned))

		cloned = expected.Clone()
		cloned.RepaymentDivertedTo = nil
		assert.Equal(t, commitment.ErrInvalidCommitment, s.Save(ctx, cloned))

		cloned = expected.Clone()
		cloned.RepaymentDivertedTo = &otherCommitmentVault2
		assert.Equal(t, commitment.ErrInvalidCommitment, s.Save(ctx, cloned))

		cloned = expected.Clone()
		assert.NoError(t, s.Save(ctx, cloned))

		actual, err := s.GetByAddress(ctx, expected.Address)
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)
	})
}

func testGetAllByState(t *testing.T, s commitment.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 10, query.Ascending)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		expected := []*commitment.Record{
			{DataVersion: splitter_token.DataVersion1, Address: "commitment1", Vault: "vault1", Pool: "pool", RecentRoot: "root", Transcript: "transcript1", Intent: "intent", ActionId: 0, Owner: "owner1", Destination: "destination", Amount: 123, State: commitment.StateOpen},
			{DataVersion: splitter_token.DataVersion1, Address: "commitment2", Vault: "vault2", Pool: "pool", RecentRoot: "root", Transcript: "transcript2", Intent: "intent", ActionId: 1, Owner: "owner2", Destination: "destination", Amount: 123, State: commitment.StateOpen},
			{DataVersion: splitter_token.DataVersion1, Address: "commitment3", Vault: "vault3", Pool: "pool", RecentRoot: "root", Transcript: "transcript3", Intent: "intent", ActionId: 2, Owner: "owner3", Destination: "destination", Amount: 123, State: commitment.StateOpen},
			{DataVersion: splitter_token.DataVersion1, Address: "commitment4", Vault: "vault4", Pool: "pool", RecentRoot: "root", Transcript: "transcript4", Intent: "intent", ActionId: 3, Owner: "owner4", Destination: "destination", Amount: 123, State: commitment.StateClosed},
			{DataVersion: splitter_token.DataVersion1, Address: "commitment5", Vault: "vault5", Pool: "pool", RecentRoot: "root", Transcript: "transcript5", Intent: "intent", ActionId: 4, Owner: "owner5", Destination: "destination", Amount: 123, State: commitment.StateClosed},
		}
		for _, record := range expected {
			require.NoError(t, s.Save(ctx, record))
		}

		_, err = s.GetAllByState(ctx, commitment.StateUnknown, query.EmptyCursor, 10, query.Ascending)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		actual, err := s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		assert.Len(t, actual, 3)

		actual, err = s.GetAllByState(ctx, commitment.StateClosed, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		assert.Len(t, actual, 2)

		// Check items (asc)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assert.Equal(t, "commitment1", actual[0].Address)
		assert.Equal(t, "commitment2", actual[1].Address)
		assert.Equal(t, "commitment3", actual[2].Address)

		// Check items (desc)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assert.Equal(t, "commitment3", actual[0].Address)
		assert.Equal(t, "commitment2", actual[1].Address)
		assert.Equal(t, "commitment1", actual[2].Address)

		// Check items (asc + limit)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 2, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "commitment1", actual[0].Address)
		assert.Equal(t, "commitment2", actual[1].Address)

		// Check items (desc + limit)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.EmptyCursor, 2, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "commitment3", actual[0].Address)
		assert.Equal(t, "commitment2", actual[1].Address)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.ToCursor(1), 5, query.Ascending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "commitment2", actual[0].Address)
		assert.Equal(t, "commitment3", actual[1].Address)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, commitment.StateOpen, query.ToCursor(3), 5, query.Descending)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, "commitment2", actual[0].Address)
		assert.Equal(t, "commitment1", actual[1].Address)
	})
}

func testGetUpgradeableByOwner(t *testing.T, s commitment.Store) {
	t.Run("testGetUpgradeableByOwner", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetUpgradeableByOwner(ctx, "owner", 10)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		futureVault := "future-vault"
		records := []*commitment.Record{
			{State: commitment.StateUnknown, Owner: "owner1"},
			{State: commitment.StatePayingDestination, Owner: "owner1"},
			{State: commitment.StateReadyToOpen, Owner: "owner1"},
			{State: commitment.StateReadyToOpen, Owner: "owner1", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateOpening, Owner: "owner2"},
			{State: commitment.StateOpening, Owner: "owner2", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateOpen, Owner: "owner2"},
			{State: commitment.StateOpen, Owner: "owner2", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateClosing, Owner: "owner2"},
			{State: commitment.StateClosing, Owner: "owner2", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateClosed, Owner: "owner2"},
			{State: commitment.StateClosed, Owner: "owner2", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateReadyToRemoveFromMerkleTree, Owner: "owner1"},
			{State: commitment.StateReadyToRemoveFromMerkleTree, Owner: "owner1", RepaymentDivertedTo: &futureVault},
			{State: commitment.StateRemovedFromMerkleTree, Owner: "owner1"},
			{State: commitment.StateRemovedFromMerkleTree, Owner: "owner1", RepaymentDivertedTo: &futureVault},
		}

		for i, record := range records {
			// Populate data irrelevant to test
			record.DataVersion = splitter_token.DataVersion1
			record.Pool = "pool"
			record.RecentRoot = "root"
			record.Address = fmt.Sprintf("address%d", i)
			record.Vault = fmt.Sprintf("vault%d", i)
			record.RecentRoot = fmt.Sprintf("root%d", i)
			record.Transcript = fmt.Sprintf("transcript%d", i)
			record.Destination = fmt.Sprintf("destination%d", i)
			record.Amount = 100
			record.Intent = fmt.Sprintf("intent%d", i)
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, record))
		}

		_, err = s.GetUpgradeableByOwner(ctx, "owner", 10)
		assert.Equal(t, commitment.ErrCommitmentNotFound, err)

		actual, err := s.GetUpgradeableByOwner(ctx, "owner1", 10)
		require.Nil(t, err)
		require.Len(t, actual, 1)
		assert.Equal(t, records[2].Address, actual[0].Address)

		actual, err = s.GetUpgradeableByOwner(ctx, "owner2", 10)
		require.Nil(t, err)
		require.Len(t, actual, 4)
		assert.Equal(t, records[4].Address, actual[0].Address)
		assert.Equal(t, records[6].Address, actual[1].Address)
		assert.Equal(t, records[8].Address, actual[2].Address)
		assert.Equal(t, records[10].Address, actual[3].Address)

		actual, err = s.GetUpgradeableByOwner(ctx, "owner2", 2)
		require.Nil(t, err)
		require.Len(t, actual, 2)
		assert.Equal(t, records[4].Address, actual[0].Address)
		assert.Equal(t, records[6].Address, actual[1].Address)
	})
}

func testGetTreasuryPoolDeficit(t *testing.T, s commitment.Store) {
	t.Run("testGetTreasuryPoolDeficit", func(t *testing.T) {
		ctx := context.Background()

		records := []*commitment.Record{
			{State: commitment.StateUnknown},
			{State: commitment.StatePayingDestination},
			{State: commitment.StateReadyToOpen},
			{State: commitment.StateOpening},
			{State: commitment.StateOpen},
			{State: commitment.StateClosing},
			{State: commitment.StateClosed},
			{State: commitment.StateReadyToRemoveFromMerkleTree},
			{State: commitment.StateRemovedFromMerkleTree},
		}

		for i, record := range records {
			record.Pool = "pool"
			record.Amount = uint64(math.Pow10(i))

			// Populate data irrelevant to test
			record.DataVersion = splitter_token.DataVersion1
			record.Address = fmt.Sprintf("address%d", i)
			record.Vault = fmt.Sprintf("vault%d", i)
			record.RecentRoot = fmt.Sprintf("root%d", i)
			record.Transcript = fmt.Sprintf("transcript%d", i)
			record.Destination = fmt.Sprintf("destination%d", i)
			record.Intent = fmt.Sprintf("intent%d", i)
			record.Owner = fmt.Sprintf("owner%d", i)
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, record))
		}

		actual, err := s.GetUsedTreasuryPoolDeficit(ctx, "pool")
		require.NoError(t, err)
		assert.EqualValues(t, 111111110, actual)

		actual, err = s.GetTotalTreasuryPoolDeficit(ctx, "pool")
		require.NoError(t, err)
		assert.EqualValues(t, 111111111, actual)

		actual, err = s.GetUsedTreasuryPoolDeficit(ctx, "other")
		require.NoError(t, err)
		assert.EqualValues(t, 0, actual)

		for i, record := range records {
			if i%2 == 0 {
				record.TreasuryRepaid = true
				require.NoError(t, s.Save(ctx, record))
			}
		}

		actual, err = s.GetUsedTreasuryPoolDeficit(ctx, "pool")
		require.NoError(t, err)
		assert.EqualValues(t, 10101010, actual)

		actual, err = s.GetTotalTreasuryPoolDeficit(ctx, "pool")
		require.NoError(t, err)
		assert.EqualValues(t, 10101010, actual)
	})
}

func testCounts(t *testing.T, s commitment.Store) {
	t.Run("testCounts", func(t *testing.T) {
		ctx := context.Background()

		futureVault1 := "future-vault-1"
		futureVault2 := "future-vault-2"
		futureVault3 := "future-vault-3"
		records := []*commitment.Record{
			{State: commitment.StateReadyToOpen, RecentRoot: "root1", RepaymentDivertedTo: &futureVault1},
			{State: commitment.StateReadyToOpen, RecentRoot: "root1", RepaymentDivertedTo: &futureVault1},
			{State: commitment.StateReadyToOpen, RecentRoot: "root2", RepaymentDivertedTo: &futureVault2},
			{State: commitment.StateClosed, RecentRoot: "root3", RepaymentDivertedTo: &futureVault2},
			{State: commitment.StateClosed, RecentRoot: "root3", RepaymentDivertedTo: &futureVault2},
			{State: commitment.StateClosed, RecentRoot: "root3", RepaymentDivertedTo: &futureVault2},
		}

		for i, record := range records {
			// Populate data irrelevant to test
			record.Pool = "pool"
			record.Amount = 1
			record.DataVersion = splitter_token.DataVersion1
			record.Address = fmt.Sprintf("address%d", i)
			record.Vault = fmt.Sprintf("vault%d", i)
			record.Transcript = fmt.Sprintf("transcript%d", i)
			record.Destination = fmt.Sprintf("destination%d", i)
			record.Intent = fmt.Sprintf("intent%d", i)
			record.Owner = fmt.Sprintf("owner%d", i)
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, record))
		}

		count, err := s.CountByState(ctx, commitment.StateOpen)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByState(ctx, commitment.StateReadyToOpen)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByState(ctx, commitment.StateClosed)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountRepaymentsDivertedToVault(ctx, futureVault1)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountRepaymentsDivertedToVault(ctx, futureVault2)
		require.NoError(t, err)
		assert.EqualValues(t, 4, count)

		count, err = s.CountRepaymentsDivertedToVault(ctx, futureVault3)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *commitment.Record) {
	assert.Equal(t, obj1.DataVersion, obj2.DataVersion)
	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.Bump, obj2.Bump)
	assert.Equal(t, obj1.Pool, obj2.Pool)
	assert.Equal(t, obj1.PoolBump, obj2.PoolBump)
	assert.Equal(t, obj1.RecentRoot, obj2.RecentRoot)
	assert.Equal(t, obj1.Transcript, obj2.Transcript)
	assert.Equal(t, obj1.Destination, obj2.Destination)
	assert.Equal(t, obj1.Amount, obj2.Amount)
	assert.Equal(t, obj1.Vault, obj2.Vault)
	assert.Equal(t, obj1.VaultBump, obj2.VaultBump)
	assert.Equal(t, obj1.Intent, obj2.Intent)
	assert.Equal(t, obj1.ActionId, obj2.ActionId)
	assert.Equal(t, obj1.Owner, obj2.Owner)
	assert.Equal(t, obj1.TreasuryRepaid, obj2.TreasuryRepaid)
	assert.EqualValues(t, obj1.RepaymentDivertedTo, obj2.RepaymentDivertedTo)
	assert.Equal(t, obj1.State, obj2.State)
}
