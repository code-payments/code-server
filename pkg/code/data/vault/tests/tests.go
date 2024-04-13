package tests

import (
	"context"
	"testing"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func RunTests(t *testing.T, s vault.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s vault.Store){
		testRoundTrip,
		testUpdate,
		testGetAllByState,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s vault.Store) {
	ctx := context.Background()

	actual, err := s.Get(ctx, "test_public_key")
	require.Error(t, err)
	assert.Equal(t, vault.ErrKeyNotFound, err)
	assert.Nil(t, actual)

	expected := vault.Record{
		PublicKey:  "test_public_key",
		PrivateKey: "test_private_key",
		State:      vault.StateUnknown,
		CreatedAt:  time.Now(),
	}
	err = s.Save(ctx, &expected)
	require.NoError(t, err)

	actual, err = s.Get(ctx, "test_public_key")
	require.NoError(t, err)
	assert.Equal(t, expected.PublicKey, actual.PublicKey)
	assert.Equal(t, expected.PrivateKey, actual.PrivateKey)
	assert.Equal(t, expected.State, actual.State)
	assert.Equal(t, expected.CreatedAt.Unix(), actual.CreatedAt.Unix())
	assert.EqualValues(t, 1, actual.Id)
}

func testUpdate(t *testing.T, s vault.Store) {
	ctx := context.Background()

	expected := vault.Record{
		PublicKey:  "test_public_key",
		PrivateKey: "test_private_key",
		State:      vault.StateUnknown,
		CreatedAt:  time.Now(),
	}
	err := s.Save(ctx, &expected)
	require.NoError(t, err)
	assert.EqualValues(t, 1, expected.Id)

	expected.State = vault.StateDeprecated

	err = s.Save(ctx, &expected)
	require.NoError(t, err)

	actual, err := s.Get(ctx, "test_public_key")
	require.NoError(t, err)
	assert.Equal(t, expected.PublicKey, actual.PublicKey)
	assert.Equal(t, expected.PrivateKey, actual.PrivateKey)
	assert.Equal(t, expected.State, actual.State)
	assert.Equal(t, expected.CreatedAt.Unix(), actual.CreatedAt.Unix())
	assert.EqualValues(t, 1, actual.Id)
}

func testGetAllByState(t *testing.T, s vault.Store) {
	ctx := context.Background()

	expected := []vault.Record{
		{PublicKey: "t1", PrivateKey: "n1", State: vault.StateDeprecated},
		{PublicKey: "t2", PrivateKey: "n2", State: vault.StateRevoked},
		{PublicKey: "t3", PrivateKey: "n3", State: vault.StateUnknown},
		{PublicKey: "t4", PrivateKey: "n4", State: vault.StateUnknown},
		{PublicKey: "t5", PrivateKey: "n5", State: vault.StateUnknown},
		{PublicKey: "t6", PrivateKey: "n6", State: vault.StateRevoked},
	}

	for _, item := range expected {
		err := s.Save(ctx, &item)
		require.NoError(t, err)
	}

	// Simple get all by state
	actual, err := s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))

	actual, err = s.GetAllByState(ctx, vault.StateDeprecated, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))

	actual, err = s.GetAllByState(ctx, vault.StateRevoked, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))

	// Simple get all by state (reverse)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))

	actual, err = s.GetAllByState(ctx, vault.StateDeprecated, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))

	actual, err = s.GetAllByState(ctx, vault.StateRevoked, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))

	// Check items (asc)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t3", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)
	assert.Equal(t, "t5", actual[2].PublicKey)

	// Check items (desc)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t5", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)
	assert.Equal(t, "t3", actual[2].PublicKey)

	// Check items (asc + limit)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 2, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t3", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)

	// Check items (desc + limit)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.EmptyCursor, 2, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t5", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)

	// Check items (asc + cursor)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.ToCursor(1), 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t3", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)
	assert.Equal(t, "t5", actual[2].PublicKey)

	// Check items (desc + cursor)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.ToCursor(6), 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 3, len(actual))
	assert.Equal(t, "t5", actual[0].PublicKey)
	assert.Equal(t, "t4", actual[1].PublicKey)
	assert.Equal(t, "t3", actual[2].PublicKey)

	// Check items (asc + cursor)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.ToCursor(3), 5, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, "t4", actual[0].PublicKey)
	assert.Equal(t, "t5", actual[1].PublicKey)

	// Check items (desc + cursor)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.ToCursor(4), 5, query.Descending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))
	assert.Equal(t, "t3", actual[0].PublicKey)

	// Check items (asc + cursor + limit)
	actual, err = s.GetAllByState(ctx, vault.StateUnknown, query.ToCursor(3), 1, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actual))
	assert.Equal(t, "t4", actual[0].PublicKey)
}

func testGetCount(t *testing.T, s vault.Store) {
	ctx := context.Background()

	expected := []vault.Record{
		{PublicKey: "t1", PrivateKey: "n1", State: vault.StateDeprecated},
		{PublicKey: "t2", PrivateKey: "n2", State: vault.StateRevoked},
		{PublicKey: "t3", PrivateKey: "n3", State: vault.StateUnknown},
		{PublicKey: "t4", PrivateKey: "n4", State: vault.StateUnknown},
		{PublicKey: "t5", PrivateKey: "n5", State: vault.StateUnknown},
		{PublicKey: "t6", PrivateKey: "n6", State: vault.StateRevoked},
	}

	for index, item := range expected {
		count, err := s.Count(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, index, count)

		err = s.Save(ctx, &item)
		require.NoError(t, err)
	}

	count, err := s.CountByState(ctx, vault.StateAvailable)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)

	count, err = s.CountByState(ctx, vault.StateDeprecated)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	count, err = s.CountByState(ctx, vault.StateRevoked)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	count, err = s.CountByState(ctx, vault.StateUnknown)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)
}
