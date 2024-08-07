package tests

import (
	"context"
	"fmt"
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
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		record1 := &ram.Record{
			Vm:                "vm1",
			Address:           "memoryaccount1",
			Capacity:          1000,
			NumSectors:        2,
			NumPages:          50,
			PageSize:          uint8(cvm.GetVirtualAccountSizeInMemory(cvm.VirtualAccountTypeTimelock)),
			StoredAccountType: cvm.VirtualAccountTypeTimelock,
		}

		record2 := &ram.Record{
			Vm:                "vm1",
			Address:           "memoryaccount2",
			Capacity:          1000,
			NumSectors:        2,
			NumPages:          50,
			PageSize:          uint8(cvm.GetVirtualAccountSizeInMemory(cvm.VirtualAccountTypeTimelock)) / 3,
			StoredAccountType: cvm.VirtualAccountTypeTimelock,
		}

		require.NoError(t, s.InitializeMemory(ctx, record1))
		require.NoError(t, s.InitializeMemory(ctx, record2))

		assert.Equal(t, ram.ErrAlreadyInitialized, s.InitializeMemory(ctx, record1))

		_, _, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeDurableNonce, "virtualaccount")
		assert.Equal(t, ram.ErrNoFreeMemory, err)

		_, _, err = s.ReserveMemory(ctx, "vm2", cvm.VirtualAccountTypeTimelock, "virtualaccount")
		assert.Equal(t, ram.ErrNoFreeMemory, err)

		address1Indices := make(map[uint16]struct{})
		address2Indices := make(map[uint16]struct{})
		for i := 0; i < 300; i++ {
			memoryAccount, index, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock, fmt.Sprintf("virtualaccount%d", i))

			if i < 100 {
				require.NoError(t, err)
				assert.Equal(t, "memoryaccount1", memoryAccount)
				assert.True(t, index < 100)
				_, ok := address1Indices[index]
				assert.False(t, ok)
				address1Indices[index] = struct{}{}
			} else if i >= 100 && i < 124 {
				require.NoError(t, err)
				assert.Equal(t, "memoryaccount2", memoryAccount)
				assert.True(t, index < 24)
				_, ok := address2Indices[index]
				assert.False(t, ok)
				address2Indices[index] = struct{}{}
			} else {
				assert.Equal(t, ram.ErrNoFreeMemory, err)
			}
		}

		for i := 0; i < 10; i++ {
			if i == 0 {
				require.NoError(t, s.FreeMemoryByIndex(ctx, "memoryaccount1", 42))
				require.NoError(t, s.FreeMemoryByAddress(ctx, "virtualaccount66"))
			} else {
				assert.Equal(t, ram.ErrNotReserved, s.FreeMemoryByIndex(ctx, "memoryaccount1", 42))
				assert.Equal(t, ram.ErrNotReserved, s.FreeMemoryByAddress(ctx, "virtualaccount66"))
			}
		}

		memoryAccount, freedIndex1, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock, "newvirtualaccount1")
		require.NoError(t, err)
		assert.Equal(t, "memoryaccount1", memoryAccount)
		assert.True(t, freedIndex1 == 42 || freedIndex1 == 66)

		_, _, err = s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock, "newvirtualaccount1")
		assert.Equal(t, ram.ErrAddressAlreadyReserved, err)

		memoryAccount, freedIndex2, err := s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock, "newvirtualaccount2")
		require.NoError(t, err)
		assert.Equal(t, "memoryaccount1", memoryAccount)
		assert.True(t, freedIndex2 == 42 || freedIndex2 == 66)

		assert.NotEqual(t, freedIndex1, freedIndex2)

		_, _, err = s.ReserveMemory(ctx, "vm1", cvm.VirtualAccountTypeTimelock, "newvirtualaccount3")
		assert.Equal(t, ram.ErrNoFreeMemory, err)
	})
}
