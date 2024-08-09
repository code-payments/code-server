package tests

import (
	"context"
	"fmt"
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

		record := &storage.Record{
			Vm:                "vm1",
			Name:              "name1",
			Address:           "storageaccount1",
			Levels:            4,
			AvailableCapacity: storage.GetMaxCapacity(4) - 1,
			Purpose:           storage.PurposeDeletion,
		}

		assert.Equal(t, storage.ErrInvalidInitialCapacity, s.InitializeStorage(ctx, record))

		record.AvailableCapacity = storage.GetMaxCapacity(record.Levels)
		cloned := record.Clone()

		start := time.Now()
		require.NoError(t, s.InitializeStorage(ctx, record))
		assert.True(t, record.Id > 0)
		assert.True(t, record.CreatedAt.After(start))

		assert.Equal(t, storage.ErrAlreadyInitialized, s.InitializeStorage(ctx, record))

		actual, err := s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeDeletion, 1)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeDeletion, storage.GetMaxCapacity(record.Levels)+1)
		assert.Equal(t, storage.ErrNotFound, err)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm1", storage.PurposeUnknown, 1)
		assert.Equal(t, storage.ErrNotFound, err)

		_, err = s.FindAnyWithAvailableCapacity(ctx, "vm2", storage.PurposeDeletion, 1)
		assert.Equal(t, storage.ErrNotFound, err)

		_, err = s.ReserveStorage(ctx, "vm2", storage.PurposeDeletion, "virtualaccount")
		assert.Equal(t, storage.ErrNoFreeStorage, err)

		_, err = s.ReserveStorage(ctx, "vm1", storage.PurposeUnknown, "virtualaccount")
		assert.Equal(t, storage.ErrNoFreeStorage, err)

		for i := 0; i < int(storage.GetMaxCapacity(4)); i++ {
			virtualAccountAddress := fmt.Sprintf("virtualaccount%d", i)

			storageAccount, err := s.ReserveStorage(ctx, "vm1", storage.PurposeDeletion, virtualAccountAddress)
			require.NoError(t, err)
			assert.Equal(t, "storageaccount1", storageAccount)

			_, err = s.ReserveStorage(ctx, "vm1", storage.PurposeDeletion, virtualAccountAddress)
			assert.Equal(t, storage.ErrAddressAlreadyReserved, err)
		}

		_, err = s.ReserveStorage(ctx, "vm1", storage.PurposeDeletion, "newvirtualaccount")
		assert.Equal(t, storage.ErrNoFreeStorage, err)
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
