package timelock

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

type Store interface {
	// Save saves a timelock account's state
	Save(ctx context.Context, record *Record) error

	// GetByAddress gets a timelock account's state by the state address
	GetByAddress(ctx context.Context, address string) (*Record, error)

	// GetByVault gets a timelock account's state by the vault address it's locking
	GetByVault(ctx context.Context, vault string) (*Record, error)

	// GetByVaultBatch is like GetByVault, but for multiple accounts. If any one account
	// is missing, ErrTimelockNotFound is returned.
	GetByVaultBatch(ctx context.Context, vaults ...string) (map[string]*Record, error)

	// GetByDepositPda gets a timelock account's state by the deposit PDA address
	GetByDepositPda(ctx context.Context, depositPda string) (*Record, error)

	// GetBySwapPda gets a timelock account's state by the swap PDA address
	GetBySwapPda(ctx context.Context, depositPda string) (*Record, error)

	// GetAllByState gets all timelock accounts in the provided state
	GetAllByState(ctx context.Context, state timelock_token.TimelockState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetCountByState gets the count of records in the provided state
	GetCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error)
}
