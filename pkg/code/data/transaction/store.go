package transaction

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/pkg/errors"
)

var (
	ErrNotFound = errors.New("no records could be found")
	ErrExists   = errors.New("the transaction already exists")
)

type Store interface {
	// Get returns a transaction record for the given signature.
	//
	// ErrNotFound is returned if no record is found.
	Get(ctx context.Context, txId string) (*Record, error)

	// Put saves transaction data to the store.
	//
	// ErrExists is returned if a transaction with the same signature already exists.
	Put(ctx context.Context, transaction *Record) error

	// GetAllByAddress returns a list of records that match a given address.
	//
	// ErrNotFound is returned if no records are found.
	GetAllByAddress(ctx context.Context, address string, cursor uint64, limit uint, ordering query.Ordering) ([]*Record, error)

	// GetLatestByState returns the latest record for a given state.
	//
	// ErrNotFound is returned if no records are found.
	GetLatestByState(ctx context.Context, address string, state Confirmation) (*Record, error)

	// GetFirstPending returns the latest record for a given state.
	//
	// ErrNotFound is returned if no records are found.
	GetFirstPending(ctx context.Context, address string) (*Record, error)

	// GetSignaturesByState returns a list of signatures that match given a confirmation state.
	//
	// ErrNotFound is returned if no records are found.
	GetSignaturesByState(ctx context.Context, filter Confirmation, limit uint, ordering query.Ordering) ([]string, error)
}
