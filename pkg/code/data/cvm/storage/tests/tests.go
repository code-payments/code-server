package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
)

func RunTests(t *testing.T, s storage.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s storage.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s storage.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		record1 := &storage.Record{
			Vm:                "vm1",
			Name:              "name1",
			Address:           "storageaccount1",
			Levels:            4,
			AvailableCapacity: storage.GetMaxCapacity(4) - 1,
			Purpose:           storage.PurposeDeletion,
		}

		assert.Equal(t, storage.ErrInvalidInitialCapacity, s.InitializeStorage(ctx, record1))

		record1.AvailableCapacity = storage.GetMaxCapacity(record1.Levels)
		cloned := record1.Clone()

		start := time.Now()
		require.NoError(t, s.InitializeStorage(ctx, record1))
		assert.True(t, record1.Id > 0)
		assert.True(t, record1.CreatedAt.After(start))

		assert.Equal(t, storage.ErrAlreadyInitialized, s.InitializeStorage(ctx, record1))

		actual, err := s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeDeletion, 1)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeDeletion, storage.GetMaxCapacity(record1.Levels)+1)
		assert.Equal(t, storage.ErrNotFound, err)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeUnknown, 1)
		assert.Equal(t, storage.ErrNotFound, err)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm2", storage.PurposeDeletion, 1)
		assert.Equal(t, storage.ErrNotFound, err)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *storage.Record) {
	assert.Equal(t, obj1.Vm, obj2.Vm)
	assert.Equal(t, obj1.Name, obj2.Name)
	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.Levels, obj2.Levels)
	assert.Equal(t, obj1.AvailableCapacity, obj2.AvailableCapacity)
	assert.Equal(t, obj1.Purpose, obj2.Purpose)
}
