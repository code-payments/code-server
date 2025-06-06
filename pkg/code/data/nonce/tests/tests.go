package tests

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s nonce.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s nonce.Store){
		testRoundTrip,
		testUpdateHappyPath,
		testUpdateStaleRecord,
		testGetAllByState,
		testGetCount,
		testBatchClaimAvailableByPurpose,
		testBatchClaimAvailableByPurposeExpirationRandomness,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s nonce.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_address")
		require.Error(t, err)
		assert.Equal(t, nonce.ErrNonceNotFound, err)
		assert.Nil(t, actual)

		expected := nonce.Record{
			Address:             "test_address",
			Authority:           "test_authority",
			Blockhash:           "test_blockhash",
			Environment:         nonce.EnvironmentSolana,
			EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
			Purpose:             nonce.PurposeClientTransaction,
			State:               nonce.StateClaimed,
			ClaimNodeID:         pointer.String("test_claim_node_id"),
			ClaimExpiresAt:      pointer.Time(time.Now().Add(time.Hour)),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		actual, err = s.Get(ctx, "test_address")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 1, actual.Version)
	})
}

func testUpdateHappyPath(t *testing.T, s nonce.Store) {
	t.Run("testUpdateHappyPath", func(t *testing.T) {
		ctx := context.Background()

		expected := nonce.Record{
			Address:             "test_address",
			Authority:           "test_authority",
			Blockhash:           "test_blockhash",
			Environment:         nonce.EnvironmentSolana,
			EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
			Purpose:             nonce.PurposeInternalServerProcess,
		}
		cloned := expected.Clone()
		err := s.Save(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		actual, err := s.Get(ctx, "test_address")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 1, actual.Version)

		expected = actual.Clone()
		expected.State = nonce.StateClaimed
		expected.Blockhash = "test_blockhash2"
		expected.Signature = "test_signature"
		expected.ClaimNodeID = pointer.String("test_claim_node_id")
		expected.ClaimExpiresAt = pointer.Time(time.Now().Add(time.Hour))
		cloned = expected.Clone()

		err = s.Save(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 2, expected.Version)

		actual, err = s.Get(ctx, "test_address")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 2, actual.Version)
	})
}

func testUpdateStaleRecord(t *testing.T, s nonce.Store) {
	t.Run("testUpdateStaleRecord", func(t *testing.T) {
		ctx := context.Background()

		expected := nonce.Record{
			Address:             "test_address",
			Authority:           "test_authority",
			Blockhash:           "test_blockhash",
			Environment:         nonce.EnvironmentSolana,
			EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
			Purpose:             nonce.PurposeInternalServerProcess,
		}
		cloned := expected.Clone()
		err := s.Save(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		actual, err := s.Get(ctx, "test_address")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 1, actual.Version)

		stale := actual.Clone()
		stale.State = nonce.StateClaimed
		stale.Blockhash = "test_blockhash2"
		stale.Signature = "test_signature"
		stale.ClaimNodeID = pointer.String("test_claim_node_id")
		stale.ClaimExpiresAt = pointer.Time(time.Now().Add(time.Hour))
		stale.Version -= 1

		err = s.Save(ctx, &stale)
		assert.Equal(t, nonce.ErrStaleVersion, err)
		assert.EqualValues(t, 1, stale.Id)
		assert.EqualValues(t, 0, stale.Version)

		actual, err = s.Get(ctx, "test_address")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 1, actual.Version)
	})
}

func testGetAllByState(t *testing.T, s nonce.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		expected := []nonce.Record{
			{Address: "t1", Authority: "a1", Blockhash: "b1", State: nonce.StateUnknown, Signature: "s1"},
			{Address: "t2", Authority: "a2", Blockhash: "b1", State: nonce.StateInvalid, Signature: "s2"},
			{Address: "t3", Authority: "a3", Blockhash: "b1", State: nonce.StateReserved, Signature: "s3"},
			{Address: "t4", Authority: "a1", Blockhash: "b2", State: nonce.StateReserved, Signature: "s4"},
			{Address: "t5", Authority: "a2", Blockhash: "b2", State: nonce.StateReserved, Signature: "s5"},
			{Address: "t6", Authority: "a3", Blockhash: "b2", State: nonce.StateInvalid, Signature: "s6"},
		}

		for _, item := range expected {
			item.Environment = nonce.EnvironmentSolana
			item.EnvironmentInstance = nonce.EnvironmentInstanceSolanaMainnet
			item.Purpose = nonce.PurposeInternalServerProcess

			err := s.Save(ctx, &item)
			require.NoError(t, err)
		}

		// Simple get all by state
		actual, err := s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateInvalid, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Simple get all by state (reverse)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))

		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))

		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateInvalid, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))

		// Check items (asc)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)
		assert.Equal(t, "t5", actual[2].Address)

		// Check items (desc)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)
		assert.Equal(t, "t3", actual[2].Address)

		// Check items (asc + limit)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 2, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t3", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)

		// Check items (desc + limit)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.EmptyCursor, 2, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t5", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.ToCursor(1), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t3", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)
		assert.Equal(t, "t5", actual[2].Address)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.ToCursor(6), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, "t5", actual[0].Address)
		assert.Equal(t, "t4", actual[1].Address)
		assert.Equal(t, "t3", actual[2].Address)

		// Check items (asc + cursor)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.ToCursor(3), 5, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actual))
		assert.Equal(t, "t4", actual[0].Address)
		assert.Equal(t, "t5", actual[1].Address)

		// Check items (desc + cursor)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.ToCursor(4), 5, query.Descending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t3", actual[0].Address)

		// Check items (asc + cursor + limit)
		actual, err = s.GetAllByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, query.ToCursor(3), 1, query.Ascending)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, "t4", actual[0].Address)
	})
}

func testGetCount(t *testing.T, s nonce.Store) {
	t.Run("testGetCount", func(t *testing.T) {
		ctx := context.Background()

		expected := []nonce.Record{
			{Address: "t1", Authority: "a1", Blockhash: "b1", State: nonce.StateUnknown, Purpose: nonce.PurposeClientTransaction, Signature: "s1"},
			{Address: "t2", Authority: "a2", Blockhash: "b1", State: nonce.StateInvalid, Purpose: nonce.PurposeClientTransaction, Signature: "s2"},
			{Address: "t3", Authority: "a3", Blockhash: "b1", State: nonce.StateReserved, Purpose: nonce.PurposeClientTransaction, Signature: "s3"},
			{Address: "t4", Authority: "a1", Blockhash: "b2", State: nonce.StateReserved, Purpose: nonce.PurposeClientTransaction, Signature: "s4"},
			{Address: "t5", Authority: "a2", Blockhash: "b2", State: nonce.StateReserved, Purpose: nonce.PurposeInternalServerProcess, Signature: "s5"},
			{Address: "t6", Authority: "a3", Blockhash: "b2", State: nonce.StateInvalid, Purpose: nonce.PurposeClientTransaction, Signature: "s6"},
		}

		for index, item := range expected {
			item.Environment = nonce.EnvironmentSolana
			item.EnvironmentInstance = nonce.EnvironmentInstanceSolanaMainnet

			count, err := s.Count(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet)
			require.NoError(t, err)
			assert.EqualValues(t, index, count)

			err = s.Save(ctx, &item)
			require.NoError(t, err)
		}

		count, err := s.CountByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateAvailable)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateInvalid)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, nonce.PurposeClientTransaction)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReserved, nonce.PurposeInternalServerProcess)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown, nonce.PurposeClientTransaction)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown, nonce.PurposeInternalServerProcess)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func testBatchClaimAvailableByPurpose(t *testing.T, s nonce.Store) {
	t.Run("testBatchClaimAvailableByPurpose", func(t *testing.T) {
		ctx := context.Background()

		minExpiry := time.Now().Add(time.Hour).Truncate(time.Millisecond)
		maxExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)

		nonces, err := s.BatchClaimAvailableByPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 100, "my_node_id", minExpiry, maxExpiry)
		require.Equal(t, nonce.ErrNonceNotFound, err)
		require.Empty(t, nonces)

		for _, purpose := range []nonce.Purpose{
			nonce.PurposeClientTransaction,
			nonce.PurposeInternalServerProcess,
		} {
			for _, state := range []nonce.State{
				nonce.StateUnknown,
				nonce.StateAvailable,
				nonce.StateReserved,
				nonce.StateClaimed,
			} {
				for i := 0; i < 50; i++ {
					record := &nonce.Record{
						Address:             fmt.Sprintf("nonce_%s_%s_%d", purpose, state, i),
						Authority:           "authority",
						Blockhash:           "bh",
						Environment:         nonce.EnvironmentSolana,
						EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
						Purpose:             purpose,
						State:               state,
						Signature:           "",
					}
					if state == nonce.StateClaimed {
						record.ClaimNodeID = pointer.String("other_node_id")

						if i < 25 {
							record.ClaimExpiresAt = pointer.Time(time.Now().Add(-time.Hour))
						} else {
							record.ClaimExpiresAt = pointer.Time(time.Now().Add(time.Hour))
						}
					}

					require.NoError(t, s.Save(ctx, record))
				}
			}
		}

		for i := 0; i < 100; i++ {
			record := &nonce.Record{
				Address:             fmt.Sprintf("nonce_devnet_%d", i),
				Authority:           "authority",
				Blockhash:           "bh",
				Environment:         nonce.EnvironmentSolana,
				EnvironmentInstance: nonce.EnvironmentInstanceSolanaDevnet,
				Purpose:             nonce.PurposeInternalServerProcess,
				State:               nonce.StateAvailable,
				Signature:           "",
			}
			require.NoError(t, s.Save(ctx, record))

			record = &nonce.Record{
				Address:             fmt.Sprintf("nonce_cvm_%d", i),
				Authority:           "authority",
				Blockhash:           "bh",
				Environment:         nonce.EnvironmentCvm,
				EnvironmentInstance: "pubkey",
				Purpose:             nonce.PurposeClientTransaction,
				State:               nonce.StateClaimed,
				Signature:           "",
				ClaimNodeID:         pointer.String("other_node_id"),
				ClaimExpiresAt:      pointer.Time(time.Now().Add(-time.Hour)),
			}
			require.NoError(t, s.Save(ctx, record))
		}

		var claimed []*nonce.Record
		for remaining := 75; remaining > 0; {
			nonces, err = s.BatchClaimAvailableByPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 10, "my_node_d", minExpiry, maxExpiry)
			require.NoError(t, err)
			require.Len(t, nonces, min(remaining, 10))

			remaining -= len(nonces)

			for _, n := range nonces {
				actual, err := s.Get(ctx, n.Address)
				require.NoError(t, err)
				require.Equal(t, nonce.StateClaimed, actual.State)
				require.NotNil(t, actual.ClaimNodeID)
				require.NotNil(t, actual.ClaimExpiresAt)
				require.Equal(t, "my_node_d", *actual.ClaimNodeID)
				require.GreaterOrEqual(t, *actual.ClaimExpiresAt, minExpiry)
				require.LessOrEqual(t, *actual.ClaimExpiresAt, maxExpiry)
				require.Equal(t, nonce.EnvironmentSolana, actual.Environment)
				require.Equal(t, nonce.EnvironmentInstanceSolanaMainnet, actual.EnvironmentInstance)
				require.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)
				require.EqualValues(t, 2, actual.Version)

				claimed = append(claimed, actual)
			}
		}

		nonces, err = s.BatchClaimAvailableByPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 10, "my_node_id", minExpiry, maxExpiry)
		require.Equal(t, nonce.ErrNonceNotFound, err)
		require.Empty(t, nonces)

		for i := range claimed[:20] {
			claimed[i].State = nonce.StateAvailable
			claimed[i].ClaimNodeID = nil
			claimed[i].ClaimExpiresAt = nil
			s.Save(ctx, claimed[i])
		}

		nonces, err = s.BatchClaimAvailableByPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 30, "my_node_id2", minExpiry, maxExpiry)
		require.NoError(t, err)
		require.Len(t, nonces, 20)

		slices.SortFunc(claimed[:20], func(a, b *nonce.Record) int {
			return strings.Compare(a.Address, b.Address)
		})
		slices.SortFunc(nonces, func(a, b *nonce.Record) int {
			return strings.Compare(a.Address, b.Address)
		})

		for i, actual := range nonces {
			require.Equal(t, nonce.StateClaimed, actual.State)
			require.NotNil(t, actual.ClaimNodeID)
			require.NotNil(t, actual.ClaimExpiresAt)
			require.Equal(t, "my_node_id2", *actual.ClaimNodeID)
			require.GreaterOrEqual(t, *actual.ClaimExpiresAt, minExpiry)
			require.LessOrEqual(t, *actual.ClaimExpiresAt, maxExpiry)
			require.Equal(t, nonce.EnvironmentSolana, actual.Environment)
			require.Equal(t, nonce.EnvironmentInstanceSolanaMainnet, actual.EnvironmentInstance)
			require.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)
			require.Equal(t, claimed[i].Address, actual.Address)
			require.EqualValues(t, 4, actual.Version)
		}
	})
}

func testBatchClaimAvailableByPurposeExpirationRandomness(t *testing.T, s nonce.Store) {
	t.Run("testBatchClaimAvailableByPurposeExpirationRandomness", func(t *testing.T) {
		ctx := context.Background()

		min := time.Now().Add(time.Hour).Truncate(time.Millisecond)
		max := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)

		for i := 0; i < 1000; i++ {
			record := &nonce.Record{
				Address:             fmt.Sprintf("nonce_%s_%s_%d", nonce.PurposeClientTransaction, nonce.StateAvailable, i),
				Authority:           "authority",
				Blockhash:           "bh",
				Environment:         nonce.EnvironmentSolana,
				EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
				Purpose:             nonce.PurposeClientTransaction,
				State:               nonce.StateAvailable,
				Signature:           "",
			}

			require.NoError(t, s.Save(ctx, record))
		}

		nonces, err := s.BatchClaimAvailableByPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 1000, "my_node_id", min, max)
		require.NoError(t, err)
		require.Len(t, nonces, 1000)

		// To verify that we have a rough random distribution of expirations,
		// we bucket the expiration space, and compute the standard deviation.
		//
		// We then compare against the expected value with a tolerance.
		// Specifically, we know there should be 50 nonces per bucket in
		// an ideal world, and we allow for a 15% deviation on this.
		bins := make([]int64, 20)
		expected := float64(len(nonces)) / float64(len(bins))
		for _, n := range nonces {
			// Formula: bin = k(val - min) / (max-min+1)
			//
			// We use '+1' in the divisor to ensure we don't divide by zero.
			// In practive, this should produce pretty much no bias since our
			// testing ranges are large.
			bin := int(n.ClaimExpiresAt.Sub(min).Milliseconds()) * len(bins) / int(max.Sub(min).Milliseconds()+1)
			bins[bin]++
		}

		sum := 0.0
		for _, count := range bins {
			diff := float64(count) - expected
			sum += diff * diff
		}

		stdDev := math.Sqrt(sum / float64(len(bins)))
		assert.LessOrEqual(t, stdDev, 0.15*expected, "expected: %v, bins %v:", expected, bins)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *nonce.Record) {
	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.Authority, obj2.Authority)
	assert.Equal(t, obj1.Blockhash, obj2.Blockhash)
	assert.Equal(t, obj1.Environment, obj2.Environment)
	assert.Equal(t, obj1.EnvironmentInstance, obj2.EnvironmentInstance)
	assert.Equal(t, obj1.Purpose, obj2.Purpose)
	assert.Equal(t, obj1.State, obj2.State)
	assert.Equal(t, obj1.ClaimNodeID, obj2.ClaimNodeID)
	assert.Equal(t, obj1.ClaimExpiresAt == nil, obj2.ClaimExpiresAt == nil)
	if obj1.ClaimExpiresAt != nil {
		assert.Equal(t, obj1.ClaimExpiresAt.UnixMilli(), obj2.ClaimExpiresAt.UnixMilli())
	}
}
