package async_airdrop

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
)

type Integration interface {
	// GetOwnersToAirdropNow gets a set of owner accounts to airdrop right now,
	// and the amount that should be airdropped.
	GetOwnersToAirdropNow(ctx context.Context) ([]*common.Account, uint64, error)

	// OnSuccess is called when an airdrop completes
	OnSuccess(ctx context.Context, owners ...*common.Account) error
}
