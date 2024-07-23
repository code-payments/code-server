package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
)

func RunTests(t *testing.T, s nonce.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s nonce.Store){
		testRoundTrip,
		testUpdate,
		testGetAllByState,
		testGetCount,
		testGetRandomAvailableByPurpose,
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
		Address:             "test_address",
		Authority:           "test_authority",
		Blockhash:           "test_blockhash",
		Environment:         nonce.EnvironmentSolana,
		EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
		Purpose:             nonce.PurposeClientTransaction,
		State:               nonce.StateReserved,
	}
	cloned := expected.Clone()
	err = s.Save(ctx, &expected)
	require.NoError(t, err)

	actual, err = s.Get(ctx, "test_address")
	require.NoError(t, err)
	assert.Equal(t, cloned.Address, actual.Address)
	assert.Equal(t, cloned.Authority, actual.Authority)
	assert.Equal(t, cloned.Blockhash, actual.Blockhash)
	assert.Equal(t, cloned.Environment, actual.Environment)
	assert.Equal(t, cloned.EnvironmentInstance, actual.EnvironmentInstance)
	assert.Equal(t, cloned.Purpose, actual.Purpose)
	assert.Equal(t, cloned.State, actual.State)
	assert.EqualValues(t, 1, actual.Id)
}

func testUpdate(t *testing.T, s nonce.Store) {
	ctx := context.Background()

	expected := nonce.Record{
		Address:             "test_address",
		Authority:           "test_authority",
		Blockhash:           "test_blockhash",
		Environment:         nonce.EnvironmentSolana,
		EnvironmentInstance: nonce.EnvironmentInstanceSolanaMainnet,
		Purpose:             nonce.PurposeInternalServerProcess,
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
		item.Environment = nonce.EnvironmentSolana
		item.EnvironmentInstance = nonce.EnvironmentInstanceSolanaMainnet
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
		item.Environment = nonce.EnvironmentSolana
		item.EnvironmentInstance = nonce.EnvironmentInstanceSolanaMainnet

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
			} {
				for i := 0; i < 500; i++ {
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
					require.NoError(t, s.Save(ctx, record))
				}
			}
		}

		selectedByAddress := make(map[string]struct{})
		for i := 0; i < 100; i++ {
			actual, err := s.GetRandomAvailableByPurpose(ctx, nonce.PurposeClientTransaction)
			require.NoError(t, err)
			assert.Equal(t, nonce.PurposeClientTransaction, actual.Purpose)
			assert.Equal(t, nonce.StateAvailable, actual.State)
			selectedByAddress[actual.Address] = struct{}{}
		}
		assert.True(t, len(selectedByAddress) > 10)

		selectedByAddress = make(map[string]struct{})
		for i := 0; i < 100; i++ {
			actual, err := s.GetRandomAvailableByPurpose(ctx, nonce.PurposeInternalServerProcess)
			require.NoError(t, err)
			assert.Equal(t, nonce.PurposeInternalServerProcess, actual.Purpose)
			assert.Equal(t, nonce.StateAvailable, actual.State)
			selectedByAddress[actual.Address] = struct{}{}
		}
		assert.True(t, len(selectedByAddress) > 10)
	})
}
