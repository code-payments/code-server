package commitment

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrCommitmentNotFound = errors.New("commitment not found")
	ErrInvalidCommitment  = errors.New("commitment record is invalid")
)

type Store interface {
	// Save saves a commitment account's state
	Save(ctx context.Context, record *Record) error

	// GetByAddress gets a commitment account's state by its address
	GetByAddress(ctx context.Context, address string) (*Record, error)

	// GetByAction gets a commitment account's state by the action it's involved in
	GetByAction(ctx context.Context, intentId string, actionId uint32) (*Record, error)

	// GetAllByState gets all commitment accounts in the provided state
	GetAllByState(ctx context.Context, state State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetUpgradeableByOwner gets commitment records that are upgradeable and owned
	// by a provided owner account.
	GetUpgradeableByOwner(ctx context.Context, owner string, limit uint64) ([]*Record, error)

	// GetUsedTreasuryPoolDeficit gets the used deficit, in Kin quarks, to a treasury
	// pool given the associated commitments
	GetUsedTreasuryPoolDeficit(ctx context.Context, pool string) (uint64, error)

	// GetTotalTreasuryPoolDeficit gets the total deficit, in Kin quarks, to a treasury
	// pool given the associated commitments
	GetTotalTreasuryPoolDeficit(ctx context.Context, pool string) (uint64, error)

	// CountByState counts the number of commitment records in a given state
	CountByState(ctx context.Context, state State) (uint64, error)

	// CountPendingRepaymentsDivertedToCommitment counts the number of commitments whose
	// pending repayments are diverted to the provided one.
	CountPendingRepaymentsDivertedToCommitment(ctx context.Context, address string) (uint64, error)
}
