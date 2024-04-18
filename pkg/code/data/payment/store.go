package payment

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/pkg/errors"
)

var (
	ErrNotFound = errors.New("no records could be found")
	ErrExists   = errors.New("the transaction index for this signature already exists")
)

type Store interface {
	// Get finds the record for a given id
	//
	// ErrNotFound is returned if the record cannot be found
	Get(ctx context.Context, txId string, index uint32) (*Record, error)

	// GetAllForTransaction returns payment records in the store for a
	// given transaction signature.
	//
	// ErrNotFound is returned if no rows are found.
	GetAllForTransaction(ctx context.Context, txId string) ([]*Record, error)

	// GetAllForAccount returns payment records in the store for a
	// given "account" after a provided "cursor" value and limited to at most
	// "limit" results.
	//
	// ErrNotFound is returned if no rows are found.
	GetAllForAccount(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering) ([]*Record, error)

	// GetAllForAccountByType returns payment records in the store for a
	// given "account" after a provided "cursor" value and limited to at most
	// "limit" results.
	//
	// ErrNotFound is returned if no rows are found.
	GetAllForAccountByType(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering, paymentType Type) ([]*Record, error)

	// GetAllForAccountByTypeAfterBlock returns payment records in the store for a
	// given "account" after a "block" after a provided "cursor" value and limited
	// to at most "limit" results.
	//
	// ErrNotFound is returned if no rows are found.
	GetAllForAccountByTypeAfterBlock(ctx context.Context, account string, block uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType Type) ([]*Record, error)

	// GetAllForAccountByTypeWithinBlockRange returns payment records in the store
	// for a given "account" within a "block" range (lowerBound, upperBOund) after a
	// provided "cursor" value and limited to at most "limit" results.
	//
	// ErrNotFound is returned if no rows are found.
	GetAllForAccountByTypeWithinBlockRange(ctx context.Context, account string, lowerBound, upperBound uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType Type) ([]*Record, error)

	// GetExternalDepositAmount gets the total amount of Kin in quarks deposited to
	// an account via a deposit from an external account.
	GetExternalDepositAmount(ctx context.Context, account string) (uint64, error)

	// Put saves payment metadata to the store.
	//
	// ErrTransactionIndexExists is returned if a transaction with the same signature already exists.
	Put(ctx context.Context, record *Record) error

	// Update an existing record on the backend store/database
	//
	// ErrNotFound is returned if the record cannot be found
	Update(ctx context.Context, record *Record) error
}
