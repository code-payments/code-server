package tests

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func RunTests(t *testing.T, s nonce.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s nonce.Store){
		testRoundTrip,
		testUpdate,
		testUpdateInvalid,
		testGetAllByState,
		testGetCount,
		testGetRandomAvailableByPurpose,
		testBatch,
		testBatchClaimExpirationRandomness,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s nonce.Store) {
	ctx := context.Background()

	actual, err := s.Get(ctx, "test_address")
	require.Error(t, err)
	assert.Equal(t, nonce.ErrNonceNotFound, err)
	assert.Nil(t, actual)

	expected := nonce.Record{
		Address:   "test_address",
		Authority: "test_authority",
		Blockhash: "test_blockhash",
		Purpose:   nonce.PurposeClientTransaction,
	}
	err = s.Save(ctx, &expected)
	require.NoError(t, err)

	actual, err = s.Get(ctx, "test_address")
	require.NoError(t, err)
	assert.Equal(t, expected.Address, actual.Address)
	assert.Equal(t, expected.Authority, actual.Authority)
	assert.Equal(t, expected.Blockhash, actual.Blockhash)
	assert.Equal(t, expected.Purpose, actual.Purpose)
	assert.EqualValues(t, 1, actual.Id)
}

func testUpdate(t *testing.T, s nonce.Store) {
	ctx := context.Background()

	expected := nonce.Record{
		Address:   "test_address",
		Authority: "test_authority",
		Blockhash: "test_blockhash",
		Purpose:   nonce.PurposeInternalServerProcess,
	}
	err := s.Save(ctx, &expected)
	require.NoError(t, err)
	assert.EqualValues(t, 1, expected.Id)

	expected.State = nonce.StateUnknown
	expected.Signature = "test_signature"

	err = s.Save(ctx, &expected)
	require.NoError(t, err)

	actual, err := s.Get(ctx, "test_address")
	require.NoError(t, err)
	assert.Equal(t, expected.Address, actual.Address)
	assert.Equal(t, expected.Authority, actual.Authority)
	assert.Equal(t, expected.Blockhash, actual.Blockhash)
	assert.Equal(t, expected.Purpose, actual.Purpose)
	assert.Equal(t, expected.State, actual.State)
	assert.Equal(t, expected.Signature, actual.Signature)
	assert.EqualValues(t, 1, actual.Id)
}

func testUpdateInvalid(t *testing.T, s nonce.Store) {
	ctx := context.Background()

	for _, invalid := range []*nonce.Record{
		{},
		{
			Address: "test_address",
		},
		{
			Address:   "test_address",
			Authority: "test_authority",
		},
		{
			Address:   "test_address",
			Authority: "test_authority",
			Blockhash: "block_hash",
		},
		{
			Address:   "test_address",
			Authority: "test_authority",
			Blockhash: "test_blockhash",
			Purpose:   nonce.PurposeClientTransaction,
			State:     nonce.StateClaimed,
		},
		{
			Address:     "test_address",
			Authority:   "test_authority",
			Blockhash:   "test_blockhash",
			Purpose:     nonce.PurposeClientTransaction,
			State:       nonce.StateClaimed,
			ClaimNodeId: "my-node",
		},
		{
			Address:        "test_address",
			Authority:      "test_authority",
			Blockhash:      "test_blockhash",
			Purpose:        nonce.PurposeClientTransaction,
			State:          nonce.StateClaimed,
			ClaimExpiresAt: time.Now().Add(time.Hour),
		},
	} {
		require.Error(t, invalid.Validate())
		assert.Error(t, s.Save(ctx, invalid))
	}
}

func testGetAllByState(t *testing.T, s nonce.Store) {
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
		item.Purpose = nonce.PurposeInternalServerProcess

		err := s.Save(ctx, &item)
		require.NoError(t, err)
	}

	// Simple get all by state
	actual, err := s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))

	actual, err = s.GetAllByState(ctx, nonce.StateUnknown, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))

	actual, err = s.GetAllByState(ctx, nonce.StateInvalid, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))

	// Simple get all by state (reverse)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))

	actual, err = s.GetAllByState(ctx, nonce.StateUnknown, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))

	actual, err = s.GetAllByState(ctx, nonce.StateInvalid, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))

	// Check items (asc)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t3", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)
	assert.Equal(t, "t5", actual[2].Address)

	// Check items (desc)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t5", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)
	assert.Equal(t, "t3", actual[2].Address)

	// Check items (asc + limit)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 2, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t3", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)

	// Check items (desc + limit)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.EmptyCursor, 2, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t5", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)

	// Check items (asc + cursor)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.ToCursor(1), 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t3", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)
	assert.Equal(t, "t5", actual[2].Address)

	// Check items (desc + cursor)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.ToCursor(6), 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t5", actual[0].Address)
	assert.Equal(t, "t4", actual[1].Address)
	assert.Equal(t, "t3", actual[2].Address)

	// Check items (asc + cursor)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.ToCursor(3), 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t4", actual[0].Address)
	assert.Equal(t, "t5", actual[1].Address)

	// Check items (desc + cursor)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.ToCursor(4), 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))
	assert.Equal(t, "t3", actual[0].Address)

	// Check items (asc + cursor + limit)
	actual, err = s.GetAllByState(ctx, nonce.StateReserved, query.ToCursor(3), 1, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))
	assert.Equal(t, "t4", actual[0].Address)
}

func testGetCount(t *testing.T, s nonce.Store) {
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
		count, err := s.Count(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, index, count)

		err = s.Save(ctx, &item)
		require.NoError(t, err)
	}

	count, err := s.CountByState(ctx, nonce.StateAvailable)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)

	count, err = s.CountByState(ctx, nonce.StateUnknown)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	count, err = s.CountByState(ctx, nonce.StateInvalid)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	count, err = s.CountByState(ctx, nonce.StateReserved)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

	count, err = s.CountByStateAndPurpose(ctx, nonce.StateReserved, nonce.PurposeClientTransaction)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	count, err = s.CountByStateAndPurpose(ctx, nonce.StateReserved, nonce.PurposeInternalServerProcess)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	count, err = s.CountByStateAndPurpose(ctx, nonce.StateUnknown, nonce.PurposeClientTransaction)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	count, err = s.CountByStateAndPurpose(ctx, nonce.StateUnknown, nonce.PurposeInternalServerProcess)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}

func testGetRandomAvailableByPurpose(t *testing.T, s nonce.Store) {
	t.Run("testGetRandomAvailableByPurpose", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetRandomAvailableByPurpose(ctx, nonce.PurposeClientTransaction)
		assert.Equal(t, nonce.ErrNonceNotFound, err)

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
						Address:   fmt.Sprintf("nonce_%s_%s_%d", purpose, state, i),
						Authority: "authority",
						Blockhash: "bh",
						Purpose:   purpose,
						State:     state,
						Signature: "",
					}
					if state == nonce.StateClaimed {
						record.ClaimNodeId = "my-node-id"

						if i < 25 {
							record.ClaimExpiresAt = time.Now().Add(-time.Hour)
						} else {
							record.ClaimExpiresAt = time.Now().Add(time.Hour)
						}
					}

					require.NoError(t, s.Save(ctx, record))
				}
			}
		}

		var sequentialLoads int
		var availableState, claimedState int
		var lastNonce *nonce.Record
		selectedByAddress := make(map[string]struct{})
		for i := 0; i < 1000; i++ {
			actual, err := s.GetRandomAvailableByPurpose(ctx, nonce.PurposeClientTransaction)
			require.NoError(t, err)
			assert.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)
			assert.True(t, actual.IsAvailable())

			switch actual.State {
			case nonce.StateAvailable:
				availableState++
			case nonce.StateClaimed:
				claimedState++
				assert.True(t, time.Now().After(actual.ClaimExpiresAt))
			default:
			}

			// We test for randomness by ensuring we're not loading nonce's sequentially.
			if lastNonce != nil && lastNonce.Purpose == actual.Purpose {
				lastID, err := strconv.ParseInt(strings.Split(lastNonce.Address, "_")[4], 10, 64)
				require.NoError(t, err)
				currentID, _ := strconv.ParseInt(strings.Split(actual.Address, "_")[4], 10, 64)
				require.NoError(t, err)

				if currentID == lastID+1 {
					sequentialLoads++
				}
			}

			selectedByAddress[actual.Address] = struct{}{}
			lastNonce = actual
		}
		assert.Greater(t, len(selectedByAddress), 10)
		assert.NotZero(t, availableState)
		assert.NotZero(t, claimedState)

		// We allocated 50 available nonce's, and 25 expired claim nonces. Given that
		// we randomly select out of the first available 100 nonces, we expect a ratio
		// of 2:1 Available vs Expired Claimed nonces.
		assert.InDelta(t, 2.0, float64(availableState)/float64(claimedState), 0.5)

		assert.Less(t, sequentialLoads, 100)

		availableState, claimedState = 0, 0
		selectedByAddress = make(map[string]struct{})
		for i := 0; i < 100; i++ {
			actual, err := s.GetRandomAvailableByPurpose(ctx, nonce.PurposeInternalServerProcess)
			require.NoError(t, err)
			assert.Equal(t, nonce.PurposeInternalServerProcess, actual.Purpose)
			assert.True(t, actual.IsAvailable())

			switch actual.State {
			case nonce.StateAvailable:
				availableState++
			case nonce.StateClaimed:
				claimedState++
				assert.True(t, time.Now().After(actual.ClaimExpiresAt))
			default:
			}

			selectedByAddress[actual.Address] = struct{}{}
		}
		assert.True(t, len(selectedByAddress) > 10)
	})
}

func testBatch(t *testing.T, s nonce.Store) {
	t.Run("testBatch", func(t *testing.T) {
		ctx := context.Background()

		minExpiry := time.Now().Add(time.Hour).Truncate(time.Millisecond)
		maxExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)

		nonces, err := s.BatchClaimAvailableByPurpose(
			ctx,
			nonce.PurposeClientTransaction,
			100,
			"my-id",
			minExpiry,
			maxExpiry,
		)
		require.Empty(t, nonces)
		require.Nil(t, err)

		// Initialize nonce pool.
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
						Address:   fmt.Sprintf("nonce_%s_%s_%d", purpose, state, i),
						Authority: "authority",
						Blockhash: "bh",
						Purpose:   purpose,
						State:     state,
						Signature: "",
					}
					if state == nonce.StateClaimed {
						record.ClaimNodeId = "my-node-id"

						if i < 25 {
							record.ClaimExpiresAt = time.Now().Add(-time.Hour)
						} else {
							record.ClaimExpiresAt = time.Now().Add(time.Hour)
						}
					}

					require.NoError(t, s.Save(ctx, record))
				}
			}
		}

		// Iteratively grab a subset until there are none left.
		//
		// Note: The odd amount ensures we try grabbing more than exists.
		var claimed []*nonce.Record
		for remaining := 75; remaining > 0; {
			nonces, err = s.BatchClaimAvailableByPurpose(
				ctx,
				nonce.PurposeClientTransaction,
				10,
				"my-id",
				minExpiry,
				maxExpiry,
			)
			require.NoError(t, err)
			require.Len(t, nonces, min(remaining, 10))

			remaining -= len(nonces)

			for _, n := range nonces {
				actual, err := s.Get(ctx, n.Address)
				require.NoError(t, err)
				require.Equal(t, nonce.StateClaimed, actual.State)
				require.Equal(t, "my-id", actual.ClaimNodeId)
				require.GreaterOrEqual(t, actual.ClaimExpiresAt, minExpiry)
				require.LessOrEqual(t, actual.ClaimExpiresAt, maxExpiry)
				require.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)

				claimed = append(claimed, actual)
			}
		}

		// Ensure no more nonces.
		nonces, err = s.BatchClaimAvailableByPurpose(ctx, nonce.PurposeClientTransaction, 10, "my-id", minExpiry, maxExpiry)
		require.NoError(t, err)
		require.Empty(t, nonces)

		// Release and reclaim
		for i := range claimed[:20] {
			claimed[i].State = nonce.StateAvailable
			s.Save(ctx, claimed[i])
		}

		nonces, err = s.BatchClaimAvailableByPurpose(ctx, nonce.PurposeClientTransaction, 30, "my-id2", minExpiry, maxExpiry)
		require.NoError(t, err)
		require.Len(t, nonces, 20)

		// We sort the sets so we can trivially compare and ensure
		// that it's the same set.
		slices.SortFunc(claimed[:20], func(a, b *nonce.Record) int {
			return strings.Compare(a.Address, b.Address)
		})
		slices.SortFunc(nonces, func(a, b *nonce.Record) int {
			return strings.Compare(a.Address, b.Address)
		})

		for i, actual := range nonces {
			require.Equal(t, nonce.StateClaimed, actual.State)
			require.Equal(t, "my-id2", actual.ClaimNodeId)
			require.GreaterOrEqual(t, actual.ClaimExpiresAt, minExpiry)
			require.LessOrEqual(t, actual.ClaimExpiresAt, maxExpiry)
			require.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)
			require.Equal(t, claimed[i].Address, actual.Address)
		}
	})
}

func testBatchClaimExpirationRandomness(t *testing.T, s nonce.Store) {
	t.Run("testBatch", func(t *testing.T) {
		ctx := context.Background()

		min := time.Now().Add(time.Hour).Truncate(time.Millisecond)
		max := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)

		for i := 0; i < 1000; i++ {
			record := &nonce.Record{
				Address:   fmt.Sprintf("nonce_%s_%s_%d", nonce.PurposeClientTransaction, nonce.StateAvailable, i),
				Authority: "authority",
				Blockhash: "bh",
				Purpose:   nonce.PurposeClientTransaction,
				State:     nonce.StateAvailable,
				Signature: "",
			}

			require.NoError(t, s.Save(ctx, record))
		}

		nonces, err := s.BatchClaimAvailableByPurpose(
			ctx,
			nonce.PurposeClientTransaction,
			1000,
			"my-id",
			min,
			max,
		)
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
