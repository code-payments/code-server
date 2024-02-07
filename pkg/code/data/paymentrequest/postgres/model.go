package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	requestTableName = "codewallet__core_paymentrequest"
	feesTableName    = "codewallet__core_paymentrequestfees"
)

type requestModel struct {
	Id sql.NullInt64 `db:"id"`

	Intent string `db:"intent"`

	DestinationTokenAccount sql.NullString  `db:"destination_token_account"`
	ExchangeCurrency        sql.NullString  `db:"exchange_currency"`
	NativeAmount            sql.NullFloat64 `db:"native_amount"`
	ExchangeRate            sql.NullFloat64 `db:"exchange_rate"`
	Quantity                sql.NullInt64   `db:"quantity"`
	Fees                    []*feeModel

	Domain     sql.NullString `db:"domain"`
	IsVerified bool           `db:"is_verified"`

	CreatedAt time.Time `db:"created_at"`
}

type feeModel struct {
	Id                      sql.NullInt64 `db:"id"`
	Intent                  string        `db:"intent"`
	DestinationTokenAccount string        `db:"destination_token_account"`
}

func toRequestModel(obj *paymentrequest.Record) (*requestModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	fees := make([]*feeModel, len(obj.Fees))
	for i, fee := range obj.Fees {
		fees[i] = &feeModel{
			Intent:                  obj.Intent,
			DestinationTokenAccount: fee.DestinationTokenAccount,
		}
	}

	return &requestModel{
		Id:     sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		Intent: obj.Intent,
		DestinationTokenAccount: sql.NullString{
			Valid:  obj.DestinationTokenAccount != nil,
			String: *pointer.StringOrDefault(obj.DestinationTokenAccount, ""),
		},
		ExchangeCurrency: sql.NullString{
			Valid:  obj.ExchangeCurrency != nil,
			String: *pointer.StringOrDefault(obj.ExchangeCurrency, ""),
		},
		NativeAmount: sql.NullFloat64{
			Valid:   obj.NativeAmount != nil,
			Float64: *pointer.Float64OrDefault(obj.NativeAmount, 0),
		},
		ExchangeRate: sql.NullFloat64{
			Valid:   obj.ExchangeRate != nil,
			Float64: *pointer.Float64OrDefault(obj.ExchangeRate, 0),
		},
		Quantity: sql.NullInt64{
			Valid: obj.Quantity != nil,
			Int64: int64(*pointer.Uint64OrDefault(obj.Quantity, 0)),
		},
		Fees: fees,
		Domain: sql.NullString{
			Valid:  obj.Domain != nil,
			String: *pointer.StringOrDefault(obj.Domain, ""),
		},
		IsVerified: obj.IsVerified,
		CreatedAt:  obj.CreatedAt,
	}, nil
}

func fromRequestModel(obj *requestModel) *paymentrequest.Record {
	fees := make([]*paymentrequest.Fee, len(obj.Fees))
	for i, fee := range obj.Fees {
		fees[i] = &paymentrequest.Fee{
			DestinationTokenAccount: fee.DestinationTokenAccount,
		}
	}

	return &paymentrequest.Record{
		Id:                      uint64(obj.Id.Int64),
		Intent:                  obj.Intent,
		DestinationTokenAccount: pointer.StringIfValid(obj.DestinationTokenAccount.Valid, obj.DestinationTokenAccount.String),
		ExchangeCurrency:        pointer.StringIfValid(obj.ExchangeCurrency.Valid, obj.ExchangeCurrency.String),
		NativeAmount:            pointer.Float64IfValid(obj.NativeAmount.Valid, obj.NativeAmount.Float64),
		ExchangeRate:            pointer.Float64IfValid(obj.ExchangeRate.Valid, obj.ExchangeRate.Float64),
		Quantity:                pointer.Uint64IfValid(obj.Quantity.Valid, uint64(obj.Quantity.Int64)),
		Fees:                    fees,
		Domain:                  pointer.StringIfValid(obj.Domain.Valid, obj.Domain.String),
		IsVerified:              obj.IsVerified,
		CreatedAt:               obj.CreatedAt.UTC(),
	}
}

func (m *requestModel) dbPut(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + requestTableName + `
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

		err = pgutil.CheckUniqueViolation(err, paymentrequest.ErrPaymentRequestAlreadyExists)
		if err != nil {
			return err
		}

		for _, fee := range m.Fees {
			query := `INSERT INTO ` + feesTableName + `
				(intent, destination_token_account)
				VALUES ($1, $2)
				RETURNING id, intent, destination_token_account`

			err := tx.QueryRowxContext(
				ctx,
				query,
				fee.Intent,
				fee.DestinationTokenAccount,
			).StructScan(fee)

			err = pgutil.CheckUniqueViolation(err, paymentrequest.ErrInvalidPaymentRequest)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, intent string) (*requestModel, error) {
	res := &requestModel{}

	query := `SELECT id, intent, destination_token_account, exchange_currency, exchange_rate, native_amount, quantity, domain, is_verified, created_at FROM ` + requestTableName + `
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

	var fees []*feeModel
	query = `SELECT id, intent, destination_token_account FROM ` + feesTableName + `
			WHERE intent = $1`

	err = db.SelectContext(ctx, &fees, query, intent)
	err = pgutil.CheckNoRows(err, nil)
	if err != nil {
		return nil, err
	}

	res.Fees = fees

	return res, nil
}
