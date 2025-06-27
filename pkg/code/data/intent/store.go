package intent

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrIntentNotFound       = errors.New("no records could be found")
	ErrMultilpeIntentsFound = errors.New("multiple records found")
	ErrStaleVersion         = errors.New("intent version is stale")
)

type Store interface {
	// Save creates or updates an intent on the store.
	Save(ctx context.Context, record *Record) error

	// Get finds the intent record for a given intent ID.
	//
	// Returns ErrNotFound if no record is found.
	Get(ctx context.Context, intentID string) (*Record, error)

	// GetAllByOwner returns all records for a given owner (as both a source and destination).
	//
	// Returns ErrNotFound if no records are found.
	GetAllByOwner(ctx context.Context, owner string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetOriginalGiftCardIssuedIntent gets the original intent where a gift card
	// was issued by its vault address.
	GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*Record, error)

	// GetGiftCardClaimedIntent gets the intent where a gift card was claimed by its
	// vault address.
	GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*Record, error)

	// GetTransactedAmountForAntiMoneyLaundering gets the total transacted core mint quarks and the
	// corresponding USD market value for an owner since a timestamp.
	GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, owner string, since time.Time) (uint64, float64, error)
}
