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
	depositPdaToUserAuthorityCache = cache.NewCache(1_000_000)
)

// todo: use a bloom filter, but a caching strategy might be ok for now
func testForKnownUserAuthorityFromDepositPda(ctx context.Context, data code_data.Provider, depositPdaAccount *common.Account) (bool, *common.Account, error) {
	cached, ok := depositPdaToUserAuthorityCache.Retrieve(depositPdaAccount.PublicKey().ToBase58())
	if ok {
		userAuthorityAccountPublicKeyString := cached.(string)
		if len(userAuthorityAccountPublicKeyString) > 0 {
			userAuthorityAccount, _ := common.NewAccountFromPublicKeyString(userAuthorityAccountPublicKeyString)
			return true, userAuthorityAccount, nil
		}
		return false, nil, nil
	}

	timelockRecord, err := data.GetTimelockByDepositPda(ctx, depositPdaAccount.PublicKey().ToBase58())
	switch err {
	case timelock.ErrTimelockNotFound:
		depositPdaToUserAuthorityCache.Insert(depositPdaAccount.PublicKey().ToBase58(), "", 1)
		return false, nil, nil
	case nil:
		userAuthorityAccount, err := common.NewAccountFromPublicKeyString(timelockRecord.VaultOwner)
		if err != nil {
			return false, nil, errors.New("invalid vault owner account")
		}
		depositPdaToUserAuthorityCache.Insert(depositPdaAccount.PublicKey().ToBase58(), userAuthorityAccount.PublicKey().ToBase58(), 1)
		return true, userAuthorityAccount, nil
	default:
		return false, nil, errors.Wrap(err, "error getting timelock record")
	}
}
