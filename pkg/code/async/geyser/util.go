package async_geyser

import (
	"context"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

var (
	codeUserStatusCache = cache.NewCache(1_000_000)
)

// todo: use a bloom filter, but a caching strategy might be ok for now
func testForKnownCodeUserAccount(ctx context.Context, data code_data.Provider, vault *common.Account) (bool, error) {
	status, ok := codeUserStatusCache.Retrieve(vault.PublicKey().ToBase58())
	if ok {
		return status.(bool), nil
	}

	_, err := data.GetTimelockByVault(ctx, vault.PublicKey().ToBase58())
	switch err {
	case timelock.ErrTimelockNotFound:
		codeUserStatusCache.Insert(vault.PublicKey().ToBase58(), false, 1)
		return false, nil
	case nil:
		codeUserStatusCache.Insert(vault.PublicKey().ToBase58(), true, 1)
		return true, nil
	default:
		return false, errors.Wrap(err, "error getting timelock record")
	}
}
