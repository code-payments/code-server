package async_sequencer

import (
	"context"
	"sync"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

var (
	// Global VM memory lock
	//
	// todo: Use a distributed lock
	vmMemoryLock sync.Mutex
)

func reserveVmMemory(ctx context.Context, data code_data.Provider, vm string, accountType cvm.VirtualAccountType, address string) (*common.Account, uint16, error) {
	vmMemoryLock.Lock()
	defer vmMemoryLock.Unlock()

	memoryAccountAddress, index, err := data.ReserveVmMemory(ctx, vm, accountType, address)
	if err != nil {
		return nil, 0, err
	}

	memoryAccount, err := common.NewAccountFromPublicKeyString(memoryAccountAddress)
	if err != nil {
		return nil, 0, err
	}

	return memoryAccount, index, nil
}

// This method can be safely called multiple times, since we know "deleted" accounts
// will never be reopened or uncompressed back into memory
func onVirtualAccountDeleted(ctx context.Context, data code_data.Provider, address string) error {
	err := data.FreeVmMemoryByAddress(ctx, address)
	if err == ram.ErrNotReserved {
		return nil
	}
	return err
}
