package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/currency"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	// todo: table should be renamed to just "intent"
	intentTableName = "codewallet__core_paymentintent"
)

type intentModel struct {
	Id                      sql.NullInt64 `db:"id"`
	IntentId                string        `db:"intent_id"`
	IntentType              uint          `db:"intent_type"`
	InitiatorOwner          string        `db:"owner"` // todo: rename the DB field to initiator_owner
	Source                  string        `db:"source"`
	DestinationOwnerAccount string        `db:"destination_owner"`
	DestinationTokenAccount string        `db:"destination"` // todo: rename the DB field to be destination_token
	Quantity                uint64        `db:"quantity"`
	ExchangeCurrency        string        `db:"exchange_currency"`
	ExchangeRate            float64       `db:"exchange_rate"`
	NativeAmount            float64       `db:"native_amount"`
	UsdMarketValue          float64       `db:"usd_market_value"`
	IsWithdrawal            bool          `db:"is_withdraw"`
	IsDeposit               bool          `db:"is_deposit"`
	IsRemoteSend            bool          `db:"is_remote_send"`
	IsReturned              bool          `db:"is_returned"`
	IsIssuerVoidingGiftCard bool          `db:"is_issuer_voiding_gift_card"`
	IsMicroPayment          bool          `db:"is_micro_payment"`
	ExtendedMetadata        []byte        `db:"extended_metadata"`
	State                   uint          `db:"state"`
	CreatedAt               time.Time     `db:"created_at"`
}

func toIntentModel(obj *intent.Record) (*intentModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	m := &intentModel{
		Id:               sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		IntentId:         obj.IntentId,
		IntentType:       uint(obj.IntentType),
		InitiatorOwner:   obj.InitiatorOwnerAccount,
		ExtendedMetadata: obj.ExtendedMetadata,
		State:            uint(obj.State),
		CreatedAt:        obj.CreatedAt,
	}

	switch obj.IntentType {
	case intent.OpenAccounts:
	case intent.ExternalDeposit:
		m.DestinationOwnerAccount = obj.ExternalDepositMetadata.DestinationOwnerAccount
		m.DestinationTokenAccount = obj.ExternalDepositMetadata.DestinationTokenAccount
		m.Quantity = obj.ExternalDepositMetadata.Quantity
		m.UsdMarketValue = obj.ExternalDepositMetadata.UsdMarketValue
	case intent.SendPublicPayment:
		m.DestinationOwnerAccount = obj.SendPublicPaymentMetadata.DestinationOwnerAccount
		m.DestinationTokenAccount = obj.SendPublicPaymentMetadata.DestinationTokenAccount
		m.Quantity = obj.SendPublicPaymentMetadata.Quantity

		m.ExchangeCurrency = strings.ToLower(string(obj.SendPublicPaymentMetadata.ExchangeCurrency))
		m.ExchangeRate = obj.SendPublicPaymentMetadata.ExchangeRate
		m.NativeAmount = obj.SendPublicPaymentMetadata.NativeAmount
		m.UsdMarketValue = obj.SendPublicPaymentMetadata.UsdMarketValue

		m.IsWithdrawal = obj.SendPublicPaymentMetadata.IsWithdrawal
		m.IsRemoteSend = obj.SendPublicPaymentMetadata.IsRemoteSend
	case intent.ReceivePaymentsPublicly:
		m.Source = obj.ReceivePaymentsPubliclyMetadata.Source
		m.Quantity = obj.ReceivePaymentsPubliclyMetadata.Quantity

		m.IsRemoteSend = obj.ReceivePaymentsPubliclyMetadata.IsRemoteSend
		m.IsReturned = obj.ReceivePaymentsPubliclyMetadata.IsReturned
		m.IsIssuerVoidingGiftCard = obj.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard

		m.ExchangeCurrency = strings.ToLower(string(obj.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency))
		m.ExchangeRate = obj.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate
		m.NativeAmount = obj.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount

		m.UsdMarketValue = obj.ReceivePaymentsPubliclyMetadata.UsdMarketValue
	default:
		return nil, errors.New("unsupported intent type")
	}

	return m, nil
}

func fromIntentModel(obj *intentModel) *intent.Record {
	record := &intent.Record{
		Id:                    uint64(obj.Id.Int64),
		IntentId:              obj.IntentId,
		IntentType:            intent.Type(obj.IntentType),
		InitiatorOwnerAccount: obj.InitiatorOwner,
		ExtendedMetadata:      obj.ExtendedMetadata,
		State:                 intent.State(obj.State),
		CreatedAt:             obj.CreatedAt.UTC(),
	}

	switch record.IntentType {
	case intent.OpenAccounts:
		record.OpenAccountsMetadata = &intent.OpenAccountsMetadata{}
	case intent.ExternalDeposit:
		record.ExternalDepositMetadata = &intent.ExternalDepositMetadata{
			DestinationOwnerAccount: obj.DestinationOwnerAccount,
			DestinationTokenAccount: obj.DestinationTokenAccount,
			Quantity:                obj.Quantity,
			UsdMarketValue:          obj.UsdMarketValue,
		}
	case intent.SendPublicPayment:
		record.SendPublicPaymentMetadata = &intent.SendPublicPaymentMetadata{
			DestinationOwnerAccount: obj.DestinationOwnerAccount,
			DestinationTokenAccount: obj.DestinationTokenAccount,
			Quantity:                obj.Quantity,

			ExchangeCurrency: currency.Code(obj.ExchangeCurrency),
			ExchangeRate:     obj.ExchangeRate,
			NativeAmount:     obj.NativeAmount,
			UsdMarketValue:   obj.UsdMarketValue,

			IsWithdrawal: obj.IsWithdrawal,
			IsRemoteSend: obj.IsRemoteSend,
		}
	case intent.ReceivePaymentsPublicly:
		record.ReceivePaymentsPubliclyMetadata = &intent.ReceivePaymentsPubliclyMetadata{
			Source:   obj.Source,
			Quantity: obj.Quantity,

			IsRemoteSend:            obj.IsRemoteSend,
			IsReturned:              obj.IsReturned,
			IsIssuerVoidingGiftCard: obj.IsIssuerVoidingGiftCard,

			OriginalExchangeCurrency: currency.Code(obj.ExchangeCurrency),
			OriginalExchangeRate:     obj.ExchangeRate,
			OriginalNativeAmount:     obj.NativeAmount,

			UsdMarketValue: obj.UsdMarketValue,
		}
	}

	return record
}

func (m *intentModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + intentTableName + `
			(intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)

			ON CONFLICT (intent_id)
			DO UPDATE
				SET state = $19
				WHERE ` + intentTableName + `.intent_id = $1 

			RETURNING
				id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.IntentId,
			m.IntentType,
			m.InitiatorOwner,
			m.Source,
			m.DestinationOwnerAccount,
			m.DestinationTokenAccount,
			m.Quantity,
			m.ExchangeCurrency,
			m.ExchangeRate,
			m.NativeAmount,
			m.UsdMarketValue,
			m.IsWithdrawal,
			m.IsDeposit,
			m.IsRemoteSend,
			m.IsReturned,
			m.IsIssuerVoidingGiftCard,
			m.IsMicroPayment,
			m.ExtendedMetadata,
			m.State,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckNoRows(err, intent.ErrInvalidIntent)
	})
}

func dbGetIntent(ctx context.Context, db *sqlx.DB, intentID string) (*intentModel, error) {
	res := &intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at
		FROM ` + intentTableName + `
		WHERE intent_id = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, intentID)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}
	return res, nil
}

func dbGetAllByOwner(ctx context.Context, db *sqlx.DB, owner string, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*intentModel, error) {
	res := []*intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at
		FROM ` + intentTableName + `
		WHERE (owner = $1 OR destination_owner = $1)
	`

	opts := []any{owner}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}

	if len(res) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	return res, nil
}

func dbGetLatestByInitiatorAndType(ctx context.Context, db *sqlx.DB, intentType intent.Type, owner string) (*intentModel, error) {
	res := &intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at
		FROM ` + intentTableName + `
		WHERE owner = $1 AND intent_type = $2
		ORDER BY created_at DESC
		LIMIT 1`

	err := db.GetContext(ctx, res, query, owner, intentType)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}
	return res, nil
}

func dbGetOriginalGiftCardIssuedIntent(ctx context.Context, db *sqlx.DB, giftCardVault string) (*intentModel, error) {
	res := []*intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at
		FROM ` + intentTableName + `
		WHERE destination = $1 and intent_type = $2 AND state != $3 AND is_remote_send IS TRUE
		LIMIT 2
	`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		giftCardVault,
		intent.SendPublicPayment,
		intent.StateRevoked,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}

	if len(res) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	if len(res) > 1 {
		return nil, intent.ErrMultilpeIntentsFound
	}

	return res[0], nil
}

func dbGetGiftCardClaimedIntent(ctx context.Context, db *sqlx.DB, giftCardVault string) (*intentModel, error) {
	res := []*intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, extended_metadata, state, created_at
		FROM ` + intentTableName + `
		WHERE source = $1 and intent_type = $2 AND state != $3 AND is_remote_send IS TRUE
		LIMIT 2
	`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		giftCardVault,
		intent.ReceivePaymentsPublicly,
		intent.StateRevoked,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}

	if len(res) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	if len(res) > 1 {
		return nil, intent.ErrMultilpeIntentsFound
	}

	return res[0], nil
}
