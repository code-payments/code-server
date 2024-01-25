package async_geyser

import (
	"context"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

var (
	codeTimelockAccountStatusCache = cache.NewCache(1_000_000)
	codeSwapAccontStatusCache      = cache.NewCache(1_000_000)
)

// todo: use a bloom filter, but a caching strategy might be ok for now
func testForKnownCodeTimelockAccount(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (bool, error) {
	status, ok := codeTimelockAccountStatusCache.Retrieve(tokenAccount.PublicKey().ToBase58())
	if ok {
		return status.(bool), nil
	}

	_, err := data.GetTimelockByVault(ctx, tokenAccount.PublicKey().ToBase58())
	switch err {
	case timelock.ErrTimelockNotFound:
		codeTimelockAccountStatusCache.Insert(tokenAccount.PublicKey().ToBase58(), false, 1)
		return false, nil
	case nil:
		codeTimelockAccountStatusCache.Insert(tokenAccount.PublicKey().ToBase58(), true, 1)
		return true, nil
	default:
		return false, errors.Wrap(err, "error getting timelock record")
	}
}

// todo: use a bloom filter, but a caching strategy might be ok for now
func testForKnownCodeSwapAccount(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (bool, error) {
	status, ok := codeSwapAccontStatusCache.Retrieve(tokenAccount.PublicKey().ToBase58())
	if ok {
		return status.(bool), nil
	}

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, tokenAccount.PublicKey().ToBase58())
	switch err {
	case account.ErrAccountInfoNotFound:
		codeSwapAccontStatusCache.Insert(tokenAccount.PublicKey().ToBase58(), false, 1)
		return false, nil
	case nil:
		isSwapAccount := accountInfoRecord.AccountType == commonpb.AccountType_SWAP
		codeSwapAccontStatusCache.Insert(tokenAccount.PublicKey().ToBase58(), isSwapAccount, 1)
		return isSwapAccount, nil
	default:
		return false, errors.Wrap(err, "error getting account info record")
	}
}
