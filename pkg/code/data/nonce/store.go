package nonce

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
)

type Store interface {
	// Count returns the total count of nonce accounts.
	Count(ctx context.Context) (uint64, error)

	// CountByState returns the total count of nonce accounts in the provided state.
	CountByState(ctx context.Context, state State) (uint64, error)

	// CountByStateAndPurpose returns the total count of nonce accounts in the provided
	// state and use case
	CountByStateAndPurpose(ctx context.Context, state State, purpose Purpose) (uint64, error)

	// Save creates or updates nonce metadata in the store.
	Save(ctx context.Context, record *Record) error

	// Get finds the nonce record for a given address.
	//
	// Returns ErrNotFound if no record is found.
	Get(ctx context.Context, address string) (*Record, error)

	// GetAllByState returns nonce records in the store for a given
	// confirmation state.
	//
	// Returns ErrNotFound if no records are found.
	GetAllByState(ctx context.Context, state State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetRandomAvailableByPurpose gets a random available nonce for a purpose.
	//
	// Returns ErrNotFound if no records are found.
	GetRandomAvailableByPurpose(ctx context.Context, purpose Purpose) (*Record, error)
}
