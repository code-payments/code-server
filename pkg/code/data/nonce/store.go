package nonce

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
)

type Store interface {
	// Count returns the total count of nonce accounts within an environment instance
	Count(ctx context.Context, env Environment, instance string) (uint64, error)

	// CountByState returns the total count of nonce accounts in the provided state within
	// an environment instance
	CountByState(ctx context.Context, env Environment, instance string, state State) (uint64, error)

	// CountByStateAndPurpose returns the total count of nonce accounts in the provided
	// state and use case within an environment instance
	CountByStateAndPurpose(ctx context.Context, env Environment, instance string, state State, purpose Purpose) (uint64, error)

	// Save creates or updates nonce metadata in the store.
	Save(ctx context.Context, record *Record) error

	// Get finds the nonce record for a given address.
	//
	// Returns ErrNotFound if no record is found.
	Get(ctx context.Context, address string) (*Record, error)

	// GetAllByState returns nonce records in the store for a given confirmation state
	// within an environment intance.
	//
	// Returns ErrNotFound if no records are found.
	GetAllByState(ctx context.Context, env Environment, instance string, state State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetRandomAvailableByPurpose gets a random available nonce for a purpose within
	// an environment instance.
	//
	// Returns ErrNotFound if no records are found.
	//
	// Deprecated in favour of BatchClaimAvailableByPurpose
	GetRandomAvailableByPurpose(ctx context.Context, env Environment, instance string, purpose Purpose) (*Record, error)

	// BatchClaimAvailableByPurpose batch claims up to the specified limit.
	//
	// The returned nonces will be marked as claimed by the current node, with
	// the specified expiry date.
	//
	// Note: Implementations need not randomize the results/selection.
	// The transactional nature of the call means that any contention exists
	// on the tx level (which always occurs), and not around fighting over
	// individual nonces.
	BatchClaimAvailableByPurpose(ctx context.Context, env Environment, instance string, purpose Purpose, limit int, nodeID string, minExpireAt, maxExpireAt time.Time) ([]*Record, error)
}
