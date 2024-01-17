package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
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
	Id                      sql.NullInt64  `db:"id"`
	IntentId                string         `db:"intent_id"`
	IntentType              uint           `db:"intent_type"`
	InitiatorOwner          string         `db:"owner"` // todo: rename the DB field to initiator_owner
	Source                  string         `db:"source"`
	DestinationOwnerAccount string         `db:"destination_owner"`
	DestinationTokenAccount string         `db:"destination"` // todo: rename the DB field to be destination_token
	Quantity                uint64         `db:"quantity"`
	TreasuryPool            sql.NullString `db:"treasury_pool"`
	RecentRoot              sql.NullString `db:"recent_root"`
	ExchangeCurrency        string         `db:"exchange_currency"`
	ExchangeRate            float64        `db:"exchange_rate"`
	NativeAmount            float64        `db:"native_amount"`
	UsdMarketValue          float64        `db:"usd_market_value"`
	IsWithdrawal            bool           `db:"is_withdraw"`
	IsDeposit               bool           `db:"is_deposit"`
	IsRemoteSend            bool           `db:"is_remote_send"`
	IsReturned              bool           `db:"is_returned"`
	IsIssuerVoidingGiftCard bool           `db:"is_issuer_voiding_gift_card"`
	IsMicroPayment          bool           `db:"is_micro_payment"`
	RelationshipTo          sql.NullString `db:"relationship_to"`
	InitiatorPhoneNumber    sql.NullString `db:"phone_number"` // todo: rename the DB field to initiator_phone_number
	State                   uint           `db:"state"`
	CreatedAt               time.Time      `db:"created_at"`
}

func toIntentModel(obj *intent.Record) (*intentModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	m := &intentModel{
		Id:             sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		IntentId:       obj.IntentId,
		IntentType:     uint(obj.IntentType),
		InitiatorOwner: obj.InitiatorOwnerAccount,
		State:          uint(obj.State),
		CreatedAt:      obj.CreatedAt,
	}

	if obj.InitiatorPhoneNumber != nil {
		m.InitiatorPhoneNumber.Valid = true
		m.InitiatorPhoneNumber.String = *obj.InitiatorPhoneNumber
	}

	switch obj.IntentType {
	case intent.LegacyPayment:
		m.Source = obj.MoneyTransferMetadata.Source
		m.DestinationTokenAccount = obj.MoneyTransferMetadata.Destination
		m.Quantity = obj.MoneyTransferMetadata.Quantity

		m.ExchangeCurrency = strings.ToLower(string(obj.MoneyTransferMetadata.ExchangeCurrency))
		m.ExchangeRate = obj.MoneyTransferMetadata.ExchangeRate
		m.UsdMarketValue = obj.MoneyTransferMetadata.UsdMarketValue

		m.IsWithdrawal = obj.MoneyTransferMetadata.IsWithdrawal
	case intent.LegacyCreateAccount:
		m.Source = obj.AccountManagementMetadata.TokenAccount
	case intent.OpenAccounts:
	case intent.SendPrivatePayment:
		m.DestinationOwnerAccount = obj.SendPrivatePaymentMetadata.DestinationOwnerAccount
		m.DestinationTokenAccount = obj.SendPrivatePaymentMetadata.DestinationTokenAccount
		m.Quantity = obj.SendPrivatePaymentMetadata.Quantity

		m.ExchangeCurrency = strings.ToLower(string(obj.SendPrivatePaymentMetadata.ExchangeCurrency))
		m.ExchangeRate = obj.SendPrivatePaymentMetadata.ExchangeRate
		m.NativeAmount = obj.SendPrivatePaymentMetadata.NativeAmount
		m.UsdMarketValue = obj.SendPrivatePaymentMetadata.UsdMarketValue

		m.IsWithdrawal = obj.SendPrivatePaymentMetadata.IsWithdrawal
		m.IsRemoteSend = obj.SendPrivatePaymentMetadata.IsRemoteSend
		m.IsMicroPayment = obj.SendPrivatePaymentMetadata.IsMicroPayment
	case intent.ReceivePaymentsPrivately:
		m.Source = obj.ReceivePaymentsPrivatelyMetadata.Source
		m.Quantity = obj.ReceivePaymentsPrivatelyMetadata.Quantity
		m.IsDeposit = obj.ReceivePaymentsPrivatelyMetadata.IsDeposit

		m.UsdMarketValue = obj.ReceivePaymentsPrivatelyMetadata.UsdMarketValue
	case intent.SaveRecentRoot:
		m.TreasuryPool.Valid = true
		m.TreasuryPool.String = obj.SaveRecentRootMetadata.TreasuryPool
		m.RecentRoot.Valid = true
		m.RecentRoot.String = obj.SaveRecentRootMetadata.PreviousMostRecentRoot
	case intent.MigrateToPrivacy2022:
		m.Quantity = obj.MigrateToPrivacy2022Metadata.Quantity
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
	case intent.EstablishRelationship:
		m.RelationshipTo = sql.NullString{
			Valid:  true,
			String: obj.EstablishRelationshipMetadata.RelationshipTo,
		}
	case intent.Login:
		m.RelationshipTo = sql.NullString{
			Valid:  true,
			String: obj.LoginMetadata.App,
		}
		m.Source = obj.LoginMetadata.UserId
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
		State:                 intent.State(obj.State),
		CreatedAt:             obj.CreatedAt.UTC(),
	}

	if obj.InitiatorPhoneNumber.Valid {
		record.InitiatorPhoneNumber = &obj.InitiatorPhoneNumber.String
	}

	switch record.IntentType {
	case intent.LegacyPayment:
		record.MoneyTransferMetadata = &intent.MoneyTransferMetadata{
			Source:      obj.Source,
			Destination: obj.DestinationTokenAccount,
			Quantity:    obj.Quantity,

			ExchangeCurrency: currency.Code(obj.ExchangeCurrency),
			ExchangeRate:     obj.ExchangeRate,
			UsdMarketValue:   obj.UsdMarketValue,

			IsWithdrawal: obj.IsWithdrawal,
		}
	case intent.LegacyCreateAccount:
		record.AccountManagementMetadata = &intent.AccountManagementMetadata{
			TokenAccount: obj.Source,
		}
	case intent.OpenAccounts:
		record.OpenAccountsMetadata = &intent.OpenAccountsMetadata{}
	case intent.SendPrivatePayment:
		record.SendPrivatePaymentMetadata = &intent.SendPrivatePaymentMetadata{
			DestinationOwnerAccount: obj.DestinationOwnerAccount,
			DestinationTokenAccount: obj.DestinationTokenAccount,
			Quantity:                obj.Quantity,

			ExchangeCurrency: currency.Code(obj.ExchangeCurrency),
			ExchangeRate:     obj.ExchangeRate,
			NativeAmount:     obj.NativeAmount,
			UsdMarketValue:   obj.UsdMarketValue,

			IsWithdrawal:   obj.IsWithdrawal,
			IsRemoteSend:   obj.IsRemoteSend,
			IsMicroPayment: obj.IsMicroPayment,
		}
	case intent.ReceivePaymentsPrivately:
		record.ReceivePaymentsPrivatelyMetadata = &intent.ReceivePaymentsPrivatelyMetadata{
			Source:    obj.Source,
			Quantity:  obj.Quantity,
			IsDeposit: obj.IsDeposit,

			UsdMarketValue: obj.UsdMarketValue,
		}
	case intent.SaveRecentRoot:
		record.SaveRecentRootMetadata = &intent.SaveRecentRootMetadata{
			TreasuryPool:           obj.TreasuryPool.String,
			PreviousMostRecentRoot: obj.RecentRoot.String,
		}
	case intent.MigrateToPrivacy2022:
		record.MigrateToPrivacy2022Metadata = &intent.MigrateToPrivacy2022Metadata{
			Quantity: obj.Quantity,
		}
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
		}
	case intent.ReceivePaymentsPublicly:
		record.ReceivePaymentsPubliclyMetadata = &intent.ReceivePaymentsPubliclyMetadata{
			Source:                  obj.Source,
			Quantity:                obj.Quantity,
			IsRemoteSend:            obj.IsRemoteSend,
			IsReturned:              obj.IsReturned,
			IsIssuerVoidingGiftCard: obj.IsIssuerVoidingGiftCard,

			OriginalExchangeCurrency: currency.Code(obj.ExchangeCurrency),
			OriginalExchangeRate:     obj.ExchangeRate,
			OriginalNativeAmount:     obj.NativeAmount,

			UsdMarketValue: obj.UsdMarketValue,
		}
	case intent.EstablishRelationship:
		record.EstablishRelationshipMetadata = &intent.EstablishRelationshipMetadata{
			RelationshipTo: obj.RelationshipTo.String,
		}
	case intent.Login:
		record.LoginMetadata = &intent.LoginMetadata{
			App:    obj.RelationshipTo.String,
			UserId: obj.Source,
		}
	}

	return record
}

func (m *intentModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + intentTableName + `
			(intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)

			ON CONFLICT (intent_id)
			DO UPDATE
				SET state = $22
				WHERE ` + intentTableName + `.intent_id = $1 

			RETURNING
				id, intent_id, intent_type, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at`

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
			m.TreasuryPool,
			m.RecentRoot,
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
			m.RelationshipTo,
			m.InitiatorPhoneNumber,
			m.State,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckNoRows(err, intent.ErrInvalidIntent)
	})
}

func dbGetIntent(ctx context.Context, db *sqlx.DB, intentID string) (*intentModel, error) {
	res := &intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
		FROM ` + intentTableName + `
		WHERE intent_id = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, intentID)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}
	return res, nil
}

func dbGetLatestByInitiatorAndType(ctx context.Context, db *sqlx.DB, intentType intent.Type, owner string) (*intentModel, error) {
	res := &intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
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

// todo: fix legacy intents
func dbGetAllByOwner(ctx context.Context, db *sqlx.DB, owner string, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*intentModel, error) {
	res := []*intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
		FROM ` + intentTableName + `
		WHERE (owner = $1 OR destination_owner = $1) AND (intent_type != $2 AND intent_type != $3)
	`

	opts := []interface{}{owner, intent.LegacyPayment, intent.LegacyCreateAccount}
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

func dbGetCountForAntispam(ctx context.Context, db *sqlx.DB, intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) (uint64, error) {
	var res uint64

	// Ugh, the Go SQL implementation has a really hard time with arrays...
	statesAsString := make([]string, len(states))
	for i, state := range states {
		statesAsString[i] = strconv.Itoa(int(state))
	}

	query := fmt.Sprintf(
		`SELECT COUNT(*) FROM `+intentTableName+` WHERE phone_number = $1 AND created_at >= $2 AND intent_type = $3 AND state IN (%s)`,
		strings.Join(statesAsString, ","),
	)
	err := db.GetContext(
		ctx,
		&res,
		query,
		phoneNumber,
		since,
		intentType,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountOwnerInteractionsForAntispam(ctx context.Context, db *sqlx.DB, sourceOwner, destinationOwner string, states []intent.State, since time.Time) (uint64, error) {
	var res uint64

	// Ugh, the Go SQL implementation has a really hard time with arrays...
	statesAsString := make([]string, len(states))
	for i, state := range states {
		statesAsString[i] = strconv.Itoa(int(state))
	}

	query := fmt.Sprintf(
		`SELECT COUNT(*) FROM `+intentTableName+` WHERE owner = $1 AND destination_owner = $2 AND created_at >= $3 AND state IN (%s)`,
		strings.Join(statesAsString, ","),
	)
	err := db.GetContext(
		ctx,
		&res,
		query,
		sourceOwner,
		destinationOwner,
		since,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetTransactedAmountForAntiMoneyLaundering(ctx context.Context, db *sqlx.DB, phoneNumber string, since time.Time) (uint64, float64, error) {
	res := struct {
		TotalQuarkValue     sql.NullInt64   `db:"total_quark_value"`
		TotalUsdMarketValue sql.NullFloat64 `db:"total_usd_value"`
	}{}

	query := `SELECT SUM(quantity) AS total_quark_value, SUM(usd_market_value) AS total_usd_value FROM ` + intentTableName + `
		WHERE phone_number = $1 AND created_at >= $2 AND intent_type = $3 AND state != $4
	`
	err := db.GetContext(
		ctx,
		&res,
		query,
		phoneNumber,
		since,
		intent.SendPrivatePayment,
		intent.StateRevoked,
	)
	if err != nil {
		return 0, 0, err
	}

	if !res.TotalQuarkValue.Valid || !res.TotalUsdMarketValue.Valid {
		return 0, 0, nil
	}
	return uint64(res.TotalQuarkValue.Int64), res.TotalUsdMarketValue.Float64, nil
}

func dbGetDepositedAmountForAntiMoneyLaundering(ctx context.Context, db *sqlx.DB, phoneNumber string, since time.Time) (uint64, float64, error) {
	res := struct {
		TotalQuarkValue     sql.NullInt64   `db:"total_quark_value"`
		TotalUsdMarketValue sql.NullFloat64 `db:"total_usd_value"`
	}{}

	query := `SELECT SUM(quantity) AS total_quark_value, SUM(usd_market_value) AS total_usd_value FROM ` + intentTableName + `
		WHERE phone_number = $1 AND created_at >= $2 AND intent_type = $3 AND is_deposit AND state != $4
	`
	err := db.GetContext(
		ctx,
		&res,
		query,
		phoneNumber,
		since,
		intent.ReceivePaymentsPrivately,
		intent.StateRevoked,
	)
	if err != nil {
		return 0, 0, err
	}

	if !res.TotalQuarkValue.Valid || !res.TotalUsdMarketValue.Valid {
		return 0, 0, nil
	}
	return uint64(res.TotalQuarkValue.Int64), res.TotalUsdMarketValue.Float64, nil
}

func dbGetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, db *sqlx.DB, account string) (int64, error) {
	var res sql.NullInt64

	query := `SELECT
		(SELECT COALESCE(SUM(quantity), 0) FROM ` + intentTableName + ` WHERE destination = $1 AND intent_type = $2 AND state IN ($3, $4, $5)) -
		(SELECT COALESCE(SUM(quantity), 0) FROM ` + intentTableName + ` WHERE source = $1 AND intent_type = $2 AND state IN ($3, $4, $5));`
	err := db.GetContext(
		ctx,
		&res,
		query,
		account,
		intent.LegacyPayment,
		intent.StatePending,
		intent.StateConfirmed,
		intent.StateFailed,
	)
	if err != nil {
		return 0, err
	}

	if !res.Valid {
		return 0, nil
	}
	return res.Int64, nil
}

func dbGetLatestSaveRecentRootIntentForTreasury(ctx context.Context, db *sqlx.DB, treasury string) (*intentModel, error) {
	res := &intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
		FROM ` + intentTableName + `
		WHERE treasury_pool = $1 and intent_type = $2
		ORDER BY id DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, treasury, intent.SaveRecentRoot)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, intent.ErrIntentNotFound)
	}
	return res, nil
}

func dbGetOriginalGiftCardIssuedIntent(ctx context.Context, db *sqlx.DB, giftCardVault string) (*intentModel, error) {
	res := []*intentModel{}

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
		FROM ` + intentTableName + `
		WHERE destination = $1 and intent_type = $2 AND state != $3 AND is_remote_send IS TRUE
		LIMIT 2
	`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		giftCardVault,
		intent.SendPrivatePayment,
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

	query := `SELECT id, intent_id, intent_type, owner, source, destination_owner, destination, quantity, treasury_pool, recent_root, exchange_currency, exchange_rate, native_amount, usd_market_value, is_withdraw, is_deposit, is_remote_send, is_returned, is_issuer_voiding_gift_card, is_micro_payment, relationship_to, phone_number, state, created_at
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
