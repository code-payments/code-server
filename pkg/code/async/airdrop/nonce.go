package async_airdrop

import (
	"context"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

func (p *service) refreshNonceMemoryAccountState(ctx context.Context) error {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()

	ai, err := p.data.GetBlockchainAccountInfo(ctx, p.nonceMemoryAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
	if err != nil {
		return err
	}
	return p.nonceMemoryAccountState.Unmarshal(ai.Data)
}

func (p *service) getVdn() (*cvm.VirtualDurableNonce, uint16, error) {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()

	vaData, _ := p.nonceMemoryAccountState.Data.Read(int(p.nextNonceIndex))
	var vdn cvm.VirtualDurableNonce
	err := vdn.UnmarshalFromMemory(vaData)
	if err != nil {
		return nil, 0, err
	}
	index := p.nextNonceIndex
	p.nextNonceIndex = (p.nextNonceIndex + 1) % uint16(len(p.nonceMemoryAccountState.Data.State))
	return &vdn, index, nil
}
