package async_sequencer

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/cvm/ram"
	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

// todo: some of these utilities likely belong in a more common package

var (
	// Global VM storage and memory locks
	//
	// todo: Use a distributed lock
	vmMemoryLock  sync.Mutex
	vmStorageLock sync.Mutex
)

func reserveVmMemory(ctx context.Context, data code_data.Provider, vm *common.Account, accountType cvm.VirtualAccountType, account *common.Account) (*common.Account, uint16, error) {
	vmMemoryLock.Lock()
	defer vmMemoryLock.Unlock()

	memoryAccountAddress, index, err := data.ReserveVmMemory(ctx, vm.PublicKey().ToBase58(), accountType, account.PublicKey().ToBase58())
	if err != nil {
		return nil, 0, err
	}

	memoryAccount, err := common.NewAccountFromPublicKeyString(memoryAccountAddress)
	if err != nil {
		return nil, 0, err
	}

	return memoryAccount, index, nil
}

func reserveVmStorage(ctx context.Context, data code_data.Provider, vm *common.Account, purpose storage.Purpose, account *common.Account) (*common.Account, error) {
	vmStorageLock.Lock()
	defer vmStorageLock.Unlock()

	storageAccountAddress, err := data.ReserveVmStorage(ctx, vm.PublicKey().ToBase58(), purpose, account.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	storageAccount, err := common.NewAccountFromPublicKeyString(storageAccountAddress)
	if err != nil {
		return nil, err
	}

	return storageAccount, nil
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

func getVirtualTimelockAccountStateInMemory(ctx context.Context, vmIndexerClient indexerpb.IndexerClient, vm, owner *common.Account) (*cvm.VirtualTimelockAccount, *common.Account, uint16, error) {
	resp, err := vmIndexerClient.GetVirtualTimelockAccounts(ctx, &indexerpb.GetVirtualTimelockAccountsRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Owner:     &indexerpb.Address{Value: owner.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, nil, 0, err
	} else if resp.Result != indexerpb.GetVirtualTimelockAccountsResponse_OK {
		return nil, nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	if len(resp.Items) > 1 {
		return nil, nil, 0, errors.New("multiple results returned")
	} else if resp.Items[0].Storage.GetMemory() == nil {
		return nil, nil, 0, errors.New("account is compressed")
	}

	protoMemory := resp.Items[0].Storage.GetMemory()
	memory, err := common.NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, nil, 0, err
	}

	protoAccount := resp.Items[0].Account
	state := cvm.VirtualTimelockAccount{
		Owner: protoAccount.Owner.Value,
		Nonce: cvm.Hash(protoAccount.Nonce.Value),

		TokenBump:    uint8(protoAccount.TokenBump),
		UnlockBump:   uint8(protoAccount.UnlockBump),
		WithdrawBump: uint8(protoAccount.WithdrawBump),

		Balance: protoAccount.Balance,
		Bump:    uint8(protoAccount.Bump),
	}

	return &state, memory, uint16(protoMemory.Index), nil
}

func getVirtualDurableNonceAccountStateInMemory(ctx context.Context, vmIndexerClient indexerpb.IndexerClient, vm, nonce *common.Account) (*cvm.VirtualDurableNonce, *common.Account, uint16, error) {
	resp, err := vmIndexerClient.GetVirtualDurableNonce(ctx, &indexerpb.GetVirtualDurableNonceRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Address:   &indexerpb.Address{Value: nonce.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, nil, 0, err
	} else if resp.Result != indexerpb.GetVirtualDurableNonceResponse_OK {
		return nil, nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	protoMemory := resp.Item.Storage.GetMemory()
	if protoMemory == nil {
		return nil, nil, 0, errors.New("account is compressed")
	}

	memory, err := common.NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, nil, 0, err
	}

	protoAccount := resp.Item.Account
	state := cvm.VirtualDurableNonce{
		Address: protoAccount.Address.Value,
		Value:   cvm.Hash(protoAccount.Nonce.Value),
	}

	return &state, memory, uint16(protoMemory.Index), nil
}

func getVirtualRelayAccountStateInMemory(ctx context.Context, vmIndexerClient indexerpb.IndexerClient, vm, relay *common.Account) (*cvm.VirtualRelayAccount, *common.Account, uint16, error) {
	resp, err := vmIndexerClient.GetVirtualRelayAccount(ctx, &indexerpb.GetVirtualRelayAccountRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Address:   &indexerpb.Address{Value: relay.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, nil, 0, err
	} else if resp.Result != indexerpb.GetVirtualRelayAccountResponse_OK {
		return nil, nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	protoMemory := resp.Item.Storage.GetMemory()
	if protoMemory == nil {
		return nil, nil, 0, errors.New("account is compressed")
	}

	memory, err := common.NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, nil, 0, err
	}

	protoAccount := resp.Item.Account
	state := cvm.VirtualRelayAccount{
		Target:      protoAccount.Target.Value,
		Destination: protoAccount.Destination.Value,
	}

	return &state, memory, uint16(protoMemory.Index), nil
}

func isInternalVmTransfer(ctx context.Context, data code_data.Provider, destination *common.Account) (bool, error) {
	// We only track Timelock managed within our VM, so the presence of a record
	// is sufficient to determine internal/external transfer status
	_, err := data.GetTimelockByVault(ctx, destination.PublicKey().ToBase58())
	switch err {
	case nil:
		return true, nil
	case timelock.ErrTimelockNotFound:
		return false, nil
	default:
		return false, err
	}
}
