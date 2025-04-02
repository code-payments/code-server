package intent

import (
	"context"
)

type Store interface {
	// Save creates or updates an intent on the store.
	Save(ctx context.Context, record *Record) error

	// Get finds the intent record for a given intent ID.
	//
	// Returns ErrNotFound if no record is found.
	Get(ctx context.Context, intentID string) (*Record, error)

	// GetLatestByInitiatorAndType gets the latest record by initiating owner and intent type
	//
	// Returns ErrNotFound if no records are found.
	GetLatestByInitiatorAndType(ctx context.Context, intentType Type, owner string) (*Record, error)

	// GetOriginalGiftCardIssuedIntent gets the original intent where a gift card
	// was issued by its vault address.
	GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*Record, error)

	// GetGiftCardClaimedIntent gets the intent where a gift card was claimed by its
	// vault address.
	GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*Record, error)
}
