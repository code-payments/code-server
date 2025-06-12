package balance

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

// OptimisticVersionLock is an optimistic version lock on an account's cached
// balance, which can be paired with DB updates against balances that need to
// be protected against race conditions.
type OptimisticVersionLock struct {
	vault          *common.Account
	currentVersion uint64
}

// GetOptimisticVersionLock gets an optimistic version lock for the vault account's
// cached balance
func GetOptimisticVersionLock(ctx context.Context, data code_data.Provider, vault *common.Account) (*OptimisticVersionLock, error) {
	version, err := data.GetCachedBalanceVersion(ctx, vault.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}
	return &OptimisticVersionLock{
		vault:          vault,
		currentVersion: version,
	}, nil
}

// OnCommit is called in the DB transaction updating the account's cached balance
func (l *OptimisticVersionLock) OnCommit(ctx context.Context, data code_data.Provider) error {
	return data.AdvanceCachedBalanceVersion(ctx, l.vault.PublicKey().ToBase58(), l.currentVersion)
}
