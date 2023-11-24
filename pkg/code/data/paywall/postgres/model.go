package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/currency"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/paywall"
)

const (
	tableName = "codewallet__core_paywall"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	OwnerAccount            string `db:"owner_account"`
	DestinationTokenAccount string `db:"destination_token_account"`

	ExchangeCurrency string  `db:"exchange_currency"`
	NativeAmount     float64 `db:"native_amount"`
	RedirectUrl      string  `db:"redirect_url"`
	ShortPath        string  `db:"short_path"`

	Signature string `db:"signature"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *paywall.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	return &model{
		Id:                      sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		OwnerAccount:            obj.OwnerAccount,
		DestinationTokenAccount: obj.DestinationTokenAccount,
		ExchangeCurrency:        string(obj.ExchangeCurrency),
		NativeAmount:            obj.NativeAmount,
		RedirectUrl:             obj.RedirectUrl,
		ShortPath:               obj.ShortPath,
		Signature:               obj.Signature,
		CreatedAt:               obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *paywall.Record {
	return &paywall.Record{
		Id:                      uint64(obj.Id.Int64),
		OwnerAccount:            obj.OwnerAccount,
		DestinationTokenAccount: obj.DestinationTokenAccount,
		ExchangeCurrency:        currency.Code(obj.ExchangeCurrency),
		NativeAmount:            obj.NativeAmount,
		RedirectUrl:             obj.RedirectUrl,
		ShortPath:               obj.ShortPath,
		Signature:               obj.Signature,
		CreatedAt:               obj.CreatedAt,
	}
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(owner_account, destination_token_account, exchange_currency, native_amount, redirect_url, short_path, signature, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, owner_account, destination_token_account, exchange_currency, native_amount, redirect_url, short_path, signature, created_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.OwnerAccount,
			m.DestinationTokenAccount,
			m.ExchangeCurrency,
			m.NativeAmount,
			m.RedirectUrl,
			m.ShortPath,
			m.Signature,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, paywall.ErrPaywallExists)
	})
}

func dbGetByShortPath(ctx context.Context, db *sqlx.DB, path string) (*model, error) {
	res := &model{}

	query := `SELECT id, owner_account, destination_token_account, exchange_currency, native_amount, redirect_url, short_path, signature, created_at FROM ` + tableName + `
			WHERE short_path = $1`

	err := db.GetContext(
		ctx,
		res,
		query,
		path,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, paywall.ErrPaywallNotFound)
	}
	return res, nil
}
