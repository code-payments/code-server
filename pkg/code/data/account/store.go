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

	// GetByAuthorityAddress finds the record for a given authority account address
	GetByAuthorityAddress(ctx context.Context, address string) (*Record, error)

	// GetLatestByOwnerAddress gets the latest accounts for an owner
	GetLatestByOwnerAddress(ctx context.Context, address string) (map[commonpb.AccountType][]*Record, error)

	// GetLatestByOwnerAddressAndType gets the latest account for an owner and account type
	GetLatestByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (*Record, error)

	// GetRelationshipByOwnerAddress gets a relationship account for a given owner.
	//
	// Note: Index is always zero, so there's no concept of a "latest"
	GetRelationshipByOwnerAddress(ctx context.Context, address, relationshipTo string) (*Record, error)

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
