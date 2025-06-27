package balance

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/balance"
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

// OnNewBalanceVersion is called in the DB transaction updating the account's
// cached balance
func (l *OptimisticVersionLock) OnNewBalanceVersion(ctx context.Context, data code_data.Provider) error {
	return data.AdvanceCachedBalanceVersion(ctx, l.vault.PublicKey().ToBase58(), l.currentVersion)
}

// RequireSameBalanceVerion is called in the DB transaction requireing the
// account's cached balance not be changed
func (l *OptimisticVersionLock) RequireSameBalanceVerion(ctx context.Context, data code_data.Provider) error {
	latestVersion, err := data.GetCachedBalanceVersion(ctx, l.vault.PublicKey().ToBase58())
	if err != nil {
		return err
	}
	if latestVersion < l.currentVersion {
		return errors.New("unexpected balance version detected")
	}
	if l.currentVersion != latestVersion {
		return balance.ErrStaleCachedBalanceVersion
	}
	return nil
}

// OpenCloseStatusLock is a lock on an account's open/close status
type OpenCloseStatusLock struct {
	vault *common.Account
}

func NewOpenCloseStatusLock(vault *common.Account) *OpenCloseStatusLock {
	return &OpenCloseStatusLock{
		vault: vault,
	}
}

// OnPaymentToAccount is called in the DB transaction making a payment to the
// account that may be closed
func (l *OpenCloseStatusLock) OnPaymentToAccount(ctx context.Context, data code_data.Provider) error {
	return data.CheckNotClosedForBalanceUpdate(ctx, l.vault.PublicKey().ToBase58())
}

// OnClose is called in the DB transaction closing the account
func (l *OpenCloseStatusLock) OnClose(ctx context.Context, data code_data.Provider) error {
	return data.MarkAsClosedForBalanceUpdate(ctx, l.vault.PublicKey().ToBase58())
}
