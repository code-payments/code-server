package intent

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
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

	// GetLatestByInitiatorAndType gets the latest record by initiating owner and intent type
	//
	// Returns ErrNotFound if no records are found.
	GetLatestByInitiatorAndType(ctx context.Context, intentType Type, owner string) (*Record, error)

	// CountForAntispam gets a count of intents for antispam purposes. It calculates the
	// number of intents by type and state for a phone number since a timestamp.
	CountForAntispam(ctx context.Context, intentType Type, phoneNumber string, states []State, since time.Time) (uint64, error)

	// CountOwnerInteractionsForAntispam gets a count of intents for antispam purposes. It
	// calculates the number of times a source owner is involved in an intent with the
	// destination owner since a timestamp.
	CountOwnerInteractionsForAntispam(ctx context.Context, sourceOwner, destinationOwner string, states []State, since time.Time) (uint64, error)

	// GetTransactedAmountForAntiMoneyLaundering gets the total transacted Kin in quarks and the
	// corresponding USD market value for a phone number since a timestamp.
	GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error)

	// GetDepositedAmountForAntiMoneyLaundering gets the total deposited Kin in quarks and the
	// corresponding USD market value for a phone number since a timestamp.
	GetDepositedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error)

	// GetNetBalanceFromPrePrivacy2022Intents gets the net balance of Kin in quarks after appying
	// pre-privacy legacy payment intents when intents detailed the entirety of the payment.
	GetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, account string) (int64, error)

	// GetLatestSaveRecentRootIntentForTreasury gets the latest SaveRecentRoot intent for a treasury
	GetLatestSaveRecentRootIntentForTreasury(ctx context.Context, treasury string) (*Record, error)

	// GetOriginalGiftCardIssuedIntent gets the original intent where a gift card
	// was issued by its vault address.
	GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*Record, error)

	// GetGiftCardClaimedIntent gets the intent where a gift card was claimed by its
	// vault address.
	GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*Record, error)
}
