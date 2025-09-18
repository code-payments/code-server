package account

import (
	"context"
	"time"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
)

var (
	ErrAccountInfoNotFound = errors.New("account info not found")
	ErrAccountInfoExists   = errors.New("account info already exists")
	ErrInvalidAccountInfo  = errors.New("invalid account info")
)

type Store interface {
	// Put creates a new account info object
	Put(ctx context.Context, record *Record) error

	// Update updates an account info object
	Update(ctx context.Context, record *Record) error

	// GetByTokenAddress finds the record for a given token account address
	GetByTokenAddress(ctx context.Context, address string) (*Record, error)

	// GetByTokenAddressBatch is like GetByTokenAddress, but for multiple accounts.
	// If any one account is missing, ErrAccountInfoNotFound is returned.
	GetByTokenAddressBatch(ctx context.Context, addresses ...string) (map[string]*Record, error)

	// GetByAuthorityAddress finds the record for a given authority account address
	// keyed by mint
	GetByAuthorityAddress(ctx context.Context, address string) (map[string]*Record, error)

	// GetLatestByOwnerAddress gets the latest accounts for an owner keyed by mint
	// and type
	//
	// For account types where only 1 account can exist, the record with the latest index is returned.
	// For account types where more than 1 account can exist, all records are returend.
	GetLatestByOwnerAddress(ctx context.Context, address string) (map[string]map[commonpb.AccountType][]*Record, error)

	// GetLatestByOwnerAddressAndType gets the latest account for an owner and account type
	// keyed by mint
	//
	// Regardless if more than 1 account for the given type can exist, only the record with
	// the largest index is returned
	GetLatestByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (map[string]*Record, error)

	// GetPrioritizedRequiringDepositSync gets a set of account info objects where
	// RequiresDepositSync is true that's prioritized by DepositsLastSyncedAt
	GetPrioritizedRequiringDepositSync(ctx context.Context, limit uint64) ([]*Record, error)

	// CountRequiringDepositSync counts the number of account info objects where
	// RequiresDepositSync is true
	CountRequiringDepositSync(ctx context.Context) (uint64, error)

	// GetPrioritizedRequiringAutoReturnCheck gets a set of account info objects where
	// RequiresAutoReturnCheck is true and older than minAge that's prioritized by CreatedAt
	GetPrioritizedRequiringAutoReturnCheck(ctx context.Context, minAge time.Duration, limit uint64) ([]*Record, error)

	// CountRequiringAutoReturnCheck counts the number of account info objects where
	// RequiresAutoReturnCheck is true
	CountRequiringAutoReturnCheck(ctx context.Context) (uint64, error)
}
