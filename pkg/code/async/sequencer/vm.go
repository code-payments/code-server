package async_sequencer

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/cvm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

// todo: some of these utilities likely belong in a more common package

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
	} else if resp.Items[0].Storage.GetCompressed() != nil {
		return nil, nil, 0, errors.New("account is compressed")
	}

	memory, err := common.NewAccountFromPublicKeyBytes(resp.Items[0].Storage.GetMemory().Account.Value)
	if err != nil {
		return nil, nil, 0, err
	}

	state := cvm.VirtualTimelockAccount{
		Owner: resp.Items[0].Account.Owner.Value,
		Nonce: cvm.Hash(resp.Items[0].Account.Nonce.Value),

		TokenBump:    uint8(resp.Items[0].Account.TokenBump),
		UnlockBump:   uint8(resp.Items[0].Account.UnlockBump),
		WithdrawBump: uint8(resp.Items[0].Account.WithdrawBump),

		Balance: resp.Items[0].Account.Balance,
		Bump:    uint8(resp.Items[0].Account.Bump),
	}

	return &state, memory, uint16(resp.Items[0].Storage.GetMemory().Index), nil
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

	memory, err := common.NewAccountFromPublicKeyBytes(resp.Item.Storage.GetMemory().Account.Value)
	if err != nil {
		return nil, nil, 0, err
	}

	state := cvm.VirtualRelayAccount{
		Address:     resp.Item.Account.Address.Value,
		Commitment:  cvm.Hash(resp.Item.Account.Commitment.Value),
		RecentRoot:  cvm.Hash(resp.Item.Account.RecentRoot.Value),
		Destination: resp.Item.Account.Destination.Value,
	}

	return &state, memory, uint16(resp.Item.Storage.GetMemory().Index), nil
}
