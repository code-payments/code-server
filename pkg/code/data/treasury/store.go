package treasury

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrTreasuryPoolNotFound          = errors.New("no records could be found")
	ErrTreasuryPoolBlockhashNotFound = errors.New("treasury pool blockhash not found")
	ErrStaleTreasuryPoolState        = errors.New("treasury pool state is stale")
	ErrNegativeFunding               = errors.New("treasury pool has negative funding")
)

type Store interface {
	// Save saves a treasury pool account's state
	Save(ctx context.Context, record *Record) error

	// GetByName gets a treasury pool account by its name
	GetByName(ctx context.Context, name string) (*Record, error)

	// GetByAddress gets a treasury pool account by its address
	GetByAddress(ctx context.Context, address string) (*Record, error)

	// GetByVault gets a treasury pool account by its vault address
	GetByVault(ctx context.Context, vault string) (*Record, error)

	// GetAllByState gets all treasury pool accounts in the provided state
	GetAllByState(ctx context.Context, state PoolState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// SaveFunding saves a funding history record for a treasury pool vault
	SaveFunding(ctx context.Context, record *FundingHistoryRecord) error

	// GetTotalAvailableFunds gets the total available funds for a treasury pool's vault
	GetTotalAvailableFunds(ctx context.Context, vault string) (uint64, error)
}
