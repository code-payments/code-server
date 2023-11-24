package vault

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
)

// todo: migrate to AWS KMS or similar

type Store interface {
	// Count returns the total count of keys.
	Count(ctx context.Context) (uint64, error)

	// CountByState returns the total count of keys by state
	CountByState(ctx context.Context, state State) (uint64, error)

	// Save creates or updates the record in the store.
	Save(ctx context.Context, record *Record) error

	// Get finds the record for a given public key.
	Get(ctx context.Context, pubkey string) (*Record, error)

	// GetAllByState returns all records for a given state.
	//
	// Returns ErrKeyNotFound if no records are found.
	GetAllByState(ctx context.Context, state State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)
}
