package async_airdrop

import (
	"context"

	"github.com/pkg/errors"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
)

var (
	errNotIndexed = errors.New("virtual account is not indexed")
)

func (p *service) getVirtualTimelockAccountMemoryLocation(ctx context.Context, vm, owner *common.Account) (*common.Account, uint16, error) {
	resp, err := p.vmIndexerClient.GetVirtualTimelockAccounts(ctx, &indexerpb.GetVirtualTimelockAccountsRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Owner:     &indexerpb.Address{Value: owner.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, 0, err
	} else if resp.Result == indexerpb.GetVirtualTimelockAccountsResponse_NOT_FOUND {
		return nil, 0, errNotIndexed
	} else if resp.Result != indexerpb.GetVirtualTimelockAccountsResponse_OK {
		return nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	if len(resp.Items) > 1 {
		return nil, 0, errors.New("multiple results returned")
	} else if resp.Items[0].Storage.GetMemory() == nil {
		return nil, 0, errors.New("account is compressed")
	}

	protoMemory := resp.Items[0].Storage.GetMemory()
	memory, err := common.NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, 0, err
	}
	return memory, uint16(protoMemory.Index), nil
}
