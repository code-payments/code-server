package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/onramp"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_fiatonramppurchase"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Owner    string    `db:"owner"`
	Platform int       `db:"platform"`
	Currency string    `db:"currency"`
	Amount   float64   `db:"amount"`
	Nonce    uuid.UUID `db:"nonce"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *onramp.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Owner:     obj.Owner,
		Platform:  obj.Platform,
		Currency:  obj.Currency,
		Amount:    obj.Amount,
		Nonce:     obj.Nonce,
		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *onramp.Record {
	return &onramp.Record{
		Id:        uint64(obj.Id.Int64),
		Owner:     obj.Owner,
		Platform:  obj.Platform,
		Currency:  obj.Currency,
		Amount:    obj.Amount,
		Nonce:     obj.Nonce,
		CreatedAt: obj.CreatedAt,
	}
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(owner, platform, currency, amount, nonce, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, owner, platform, currency, amount, nonce, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Owner,
			m.Platform,
			m.Currency,
			m.Amount,
			m.Nonce,
			m.CreatedAt.UTC(),
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, onramp.ErrPurchaseAlreadyExists)
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, nonce uuid.UUID) (*model, error) {
	res := &model{}

	query := `SELECT
		id, owner, platform, currency, amount, nonce, created_at
		FROM ` + tableName + `
		WHERE nonce = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, nonce)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, onramp.ErrPurchaseNotFound)
	}
	return res, nil
}
