package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

func RunTests(t *testing.T, s ram.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s ram.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s ram.Store) {
	ctx := context.Background()

	record1 := &ram.Record{
		Vm:                "vm1",
		Address:           "address1",
		Capacity:          1000,
		NumSectors:        2,
		NumPages:          50,
		PageSize:          uint8(cvm.GetVirtualAccountSizeInMemory(cvm.VirtualAccountTypeTimelock)),
		StoredAccountType: cvm.VirtualAccountTypeTimelock,
	}

	record2 := &ram.Record{
		Vm:                "vm1",
		Address:           "address2",
		Capacity:          1000,
		NumSectors:        2,
		NumPages:          50,
		PageSize:          uint8(cvm.GetVirtualAccountSizeInMemory(cvm.VirtualAccountTypeTimelock)) / 3,
		StoredAccountType: cvm.VirtualAccountTypeTimelock,
	}

	require.NoError(t, s.InitializeMemory(ctx, record1))
	require.NoError(t, s.InitializeMemory(ctx, record2))

	assert.Equal(t, ram.ErrAlreadyInitialized, s.InitializeMemory(ctx, record1))

	_, _, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeDurableNonce)
	assert.Equal(t, ram.ErrNoFreeMemory, err)

	_, _, err = s.ReserveMemory(ctx, "vm2", cvm.VirtualAccountTypeTimelock)
	assert.Equal(t, ram.ErrNoFreeMemory, err)

	for i := 0; i < 300; i++ {
		address, index, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock)

		if i < 100 {
			require.NoError(t, err)
			assert.Equal(t, "address1", address)
			assert.EqualValues(t, i, index)
		} else if i >= 100 && i < 124 {
			require.NoError(t, err)
			assert.Equal(t, "address2", address)
			assert.EqualValues(t, i-100, index)
		} else {
			assert.Equal(t, ram.ErrNoFreeMemory, err)
		}
	}

	for i := 0; i < 10; i++ {
		if i == 0 {
			require.NoError(t, s.FreeMemory(ctx, "address1", 42))
			require.NoError(t, s.FreeMemory(ctx, "address1", 66))
		} else {
			assert.Equal(t, ram.ErrNotReserved, s.FreeMemory(ctx, "address1", 42))
		}
	}

	address, index, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock)
	require.NoError(t, err)
	assert.Equal(t, "address1", address)
	assert.EqualValues(t, 42, index)

	address, index, err = s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock)
	require.NoError(t, err)
	assert.Equal(t, "address1", address)
	assert.EqualValues(t, 66, index)

	_, _, err = s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock)
	assert.Equal(t, ram.ErrNoFreeMemory, err)
}
