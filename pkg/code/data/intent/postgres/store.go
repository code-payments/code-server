package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) intent.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save creates or updates a intnent on the store.
func (s *store) Save(ctx context.Context, record *intent.Record) error {
	obj, err := toIntentModel(record)
	if err != nil {
		return err
	}

	err = obj.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromIntentModel(obj)
	res.CopyTo(record)

	return nil
}

// Get finds the intent record for a given intent ID.
//
// Returns ErrNotFound if no record is found.
func (s *store) Get(ctx context.Context, intentID string) (*intent.Record, error) {
	obj, err := dbGetIntent(ctx, s.db, intentID)
	if err != nil {
		return nil, err
	}

	return fromIntentModel(obj), nil
}

// GetLatestByInitiatorAndType gets the latest record by intent type and initiating owner
//
// Returns ErrNotFound if no records are found.
func (s *store) GetLatestByInitiatorAndType(ctx context.Context, intentType intent.Type, owner string) (*intent.Record, error) {
	model, err := dbGetLatestByInitiatorAndType(ctx, s.db, intentType, owner)
	if err != nil {
		return nil, err
	}

	return fromIntentModel(model), nil
}

// GetAllByOwner returns all records for a given owner (as both a source and destination).
//
// Returns ErrNotFound if no records are found.
func (s *store) GetAllByOwner(ctx context.Context, owner string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*intent.Record, error) {
	models, err := dbGetAllByOwner(ctx, s.db, owner, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	intents := make([]*intent.Record, len(models))
	for i, model := range models {
		intents[i] = fromIntentModel(model)
	}

	return intents, nil
}

// CountForAntispam gets a count of intents for antispam purposes. It calculates the
// number of intents by type and state for a phone number since a timestamp.
func (s *store) CountForAntispam(ctx context.Context, intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) (uint64, error) {
	return dbGetCountForAntispam(ctx, s.db, intentType, phoneNumber, states, since)
}

// CountOwnerInteractionsForAntispam gets a count of intents for antispam purposes. It
// calculates the number of times a source owner is involved in an intent with the
// destination owner since a timestamp.
func (s *store) CountOwnerInteractionsForAntispam(ctx context.Context, sourceOwner, destinationOwner string, states []intent.State, since time.Time) (uint64, error) {
	return dbGetCountOwnerInteractionsForAntispam(ctx, s.db, sourceOwner, destinationOwner, states, since)
}

// GetTransactedAmountForAntiMoneyLaundering gets the total transacted Kin in quarks and the
// corresponding USD market value for a phone number since a timestamp.
func (s *store) GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	return dbGetTransactedAmountForAntiMoneyLaundering(ctx, s.db, phoneNumber, since)
}

// GetDepositedAmountForAntiMoneyLaundering gets the total deposited Kin in quarks and the
// corresponding USD market value for a phone number since a timestamp.
func (s *store) GetDepositedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	return dbGetDepositedAmountForAntiMoneyLaundering(ctx, s.db, phoneNumber, since)
}

// GetNetBalanceFromPrePrivacy2022Intents gets the net balance of Kin in quarks after appying
// pre-privacy legacy payment intents when intents detailed the entirety of the payment.
func (s *store) GetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, account string) (int64, error) {
	return dbGetNetBalanceFromPrePrivacy2022Intents(ctx, s.db, account)
}

// GetLatestSaveRecentRootIntentForTreasury gets the latest SaveRecentRoot intent for a treasury
func (s *store) GetLatestSaveRecentRootIntentForTreasury(ctx context.Context, treasury string) (*intent.Record, error) {
	model, err := dbGetLatestSaveRecentRootIntentForTreasury(ctx, s.db, treasury)
	if err != nil {
		return nil, err
	}

	return fromIntentModel(model), nil
}

// GetOriginalGiftCardIssuedIntent gets the original intent where a gift card
// was issued by its vault address.
func (s *store) GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	model, err := dbGetOriginalGiftCardIssuedIntent(ctx, s.db, giftCardVault)
	if err != nil {
		return nil, err
	}

	return fromIntentModel(model), nil
}

// GetGiftCardClaimedIntent gets the intent where a gift card was claimed by its
// vault address
func (s *store) GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	model, err := dbGetGiftCardClaimedIntent(ctx, s.db, giftCardVault)
	if err != nil {
		return nil, err
	}

	return fromIntentModel(model), nil
}
