package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/currency"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
)

const (
	tableName = "codewallet__core_paymentrequest"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Intent string `db:"intent"`

	DestinationTokenAccount string `db:"destination_token_account"`

	ExchangeCurrency string          `db:"exchange_currency"`
	NativeAmount     float64         `db:"native_amount"`
	ExchangeRate     sql.NullFloat64 `db:"exchange_rate"`
	Quantity         sql.NullInt64   `db:"quantity"`

	Domain     sql.NullString `db:"domain"`
	IsVerified bool           `db:"is_verified"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *paymentrequest.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	return &model{
		Id:                      sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		Intent:                  obj.Intent,
		DestinationTokenAccount: obj.DestinationTokenAccount,
		ExchangeCurrency:        string(obj.ExchangeCurrency),
		NativeAmount:            obj.NativeAmount,
		ExchangeRate: sql.NullFloat64{
			Valid:   obj.ExchangeRate != nil,
			Float64: *pointer.Float64OrDefault(obj.ExchangeRate, 0),
		},
		Quantity: sql.NullInt64{
			Valid: obj.Quantity != nil,
			Int64: int64(*pointer.Uint64OrDefault(obj.Quantity, 0)),
		},
		Domain: sql.NullString{
			Valid:  obj.Domain != nil,
			String: *pointer.StringOrDefault(obj.Domain, ""),
		},
		IsVerified: obj.IsVerified,
		CreatedAt:  obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *paymentrequest.Record {
	return &paymentrequest.Record{
		Id:                      uint64(obj.Id.Int64),
		Intent:                  obj.Intent,
		DestinationTokenAccount: obj.DestinationTokenAccount,
		ExchangeCurrency:        currency.Code(obj.ExchangeCurrency),
		NativeAmount:            obj.NativeAmount,
		ExchangeRate:            pointer.Float64IfValid(obj.ExchangeRate.Valid, obj.ExchangeRate.Float64),
		Quantity:                pointer.Uint64IfValid(obj.Quantity.Valid, uint64(obj.Quantity.Int64)),
		Domain:                  pointer.StringIfValid(obj.Domain.Valid, obj.Domain.String),
		IsVerified:              obj.IsVerified,
		CreatedAt:               obj.CreatedAt.UTC(),
	}
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(intent, destination_token_account, exchange_currency, exchange_rate, native_amount, quantity, domain, is_verified, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, intent, destination_token_account, exchange_currency, exchange_rate, native_amount, quantity, domain, is_verified, created_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Intent,
			m.DestinationTokenAccount,
			m.ExchangeCurrency,
			m.ExchangeRate,
			m.NativeAmount,
			m.Quantity,
			m.Domain,
			m.IsVerified,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, paymentrequest.ErrPaymentRequestAlreadyExists)
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, intent string) (*model, error) {
	res := &model{}

	query := `SELECT id, intent, destination_token_account, exchange_currency, exchange_rate, native_amount, quantity, domain, is_verified, created_at FROM ` + tableName + `
			WHERE intent = $1`

	err := db.GetContext(
		ctx,
		res,
		query,
		intent,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, paymentrequest.ErrPaymentRequestNotFound)
	}
	return res, nil
}
