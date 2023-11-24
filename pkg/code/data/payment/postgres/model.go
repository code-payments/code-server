package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

const (
	tableName = "codewallet__core_payment"

	tableColumns = `
		block_id,
		block_time,
		transaction_id,
		transaction_index,
		rendezvous_key,
		is_external,

		source,
		destination,
		quantity,

		exchange_currency,
		region,
		exchange_rate,
		usd_market_value,

		is_withdraw,

		confirmation_state,
		created_at
	`
)

type model struct {
	Id                sql.NullInt64            `db:"id"`
	BlockId           sql.NullInt64            `db:"block_id"`
	BlockTime         sql.NullTime             `db:"block_time"`
	TransactionId     string                   `db:"transaction_id"`
	TransactionIndex  uint32                   `db:"transaction_index"`
	Rendezvous        sql.NullString           `db:"rendezvous_key"`
	IsExternal        bool                     `db:"is_external"`
	SourceId          string                   `db:"source"`
	DestinationId     string                   `db:"destination"`
	Quantity          uint64                   `db:"quantity"`
	ExchangeCurrency  string                   `db:"exchange_currency"`
	Region            sql.NullString           `db:"region"`
	ExchangeRate      float64                  `db:"exchange_rate"`
	UsdMarketValue    float64                  `db:"usd_market_value"`
	IsWithdraw        bool                     `db:"is_withdraw"`
	ConfirmationState transaction.Confirmation `db:"confirmation_state"`
	CreatedAt         time.Time                `db:"created_at"`
}

func toModel(obj *payment.Record) *model {
	m := &model{
		Id:                sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		BlockId:           sql.NullInt64{Int64: int64(obj.BlockId), Valid: obj.BlockId > 0},
		BlockTime:         sql.NullTime{Time: obj.BlockTime, Valid: !obj.BlockTime.IsZero()},
		TransactionId:     obj.TransactionId,
		TransactionIndex:  obj.TransactionIndex,
		Rendezvous:        sql.NullString{String: obj.Rendezvous, Valid: obj.Rendezvous != ""},
		IsExternal:        obj.IsExternal,
		SourceId:          obj.Source,
		DestinationId:     obj.Destination,
		Quantity:          obj.Quantity,
		ExchangeCurrency:  strings.ToLower(obj.ExchangeCurrency),
		ExchangeRate:      obj.ExchangeRate,
		UsdMarketValue:    obj.UsdMarketValue,
		IsWithdraw:        obj.IsWithdraw,
		ConfirmationState: obj.ConfirmationState,
		CreatedAt:         obj.CreatedAt.UTC(),
	}

	if obj.Region != nil {
		m.Region.Valid = true
		m.Region.String = strings.ToLower(*obj.Region)
	}

	return m
}

func fromModel(obj *model) *payment.Record {
	record := &payment.Record{
		Id:                uint64(obj.Id.Int64),
		BlockId:           uint64(obj.BlockId.Int64),
		BlockTime:         obj.BlockTime.Time.UTC(),
		TransactionId:     obj.TransactionId,
		TransactionIndex:  obj.TransactionIndex,
		Rendezvous:        obj.Rendezvous.String,
		IsExternal:        obj.IsExternal,
		Source:            obj.SourceId,
		Destination:       obj.DestinationId,
		Quantity:          obj.Quantity,
		ExchangeCurrency:  strings.ToLower(obj.ExchangeCurrency),
		ExchangeRate:      obj.ExchangeRate,
		UsdMarketValue:    obj.UsdMarketValue,
		IsWithdraw:        obj.IsWithdraw,
		ConfirmationState: obj.ConfirmationState,
		CreatedAt:         obj.CreatedAt.UTC(),
	}

	if obj.Region.Valid {
		record.Region = &obj.Region.String
	}

	return record
}

func (self *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + ` (` + tableColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;`

	err := db.QueryRowxContext(ctx, query,
		self.BlockId,
		self.BlockTime,
		self.TransactionId,
		self.TransactionIndex,
		self.Rendezvous,
		self.IsExternal,
		self.SourceId,
		self.DestinationId,
		self.Quantity,
		self.ExchangeCurrency,
		self.Region,
		self.ExchangeRate,
		self.UsdMarketValue,
		self.IsWithdraw,
		self.ConfirmationState,
		self.CreatedAt,
	).StructScan(self)

	return pgutil.CheckUniqueViolation(err, payment.ErrExists)
}

func (self *model) dbUpdate(ctx context.Context, db *sqlx.DB) error {
	query := `UPDATE ` + tableName + `
		SET
			block_id 			= $2,
			block_time 			= $3,
			exchange_currency 	= $4,
			region              = $5,
			exchange_rate 		= $6,
			usd_market_value 	= $7,
			confirmation_state 	= $8
		WHERE id = $1 RETURNING *;`

	err := db.QueryRowxContext(ctx, query,
		self.Id,
		self.BlockId,
		self.BlockTime,
		self.ExchangeCurrency,
		self.Region,
		self.ExchangeRate,
		self.UsdMarketValue,
		self.ConfirmationState,
	).StructScan(self)

	return pgutil.CheckNoRows(err, payment.ErrNotFound)
}

func makeSelectQuery(condition string, ordering q.Ordering) string {
	return `SELECT * FROM ` + tableName + ` WHERE ` + condition + ` ORDER BY id ` + q.FromOrderingWithFallback(ordering, "asc")
}

func makeGetQuery(condition string, ordering q.Ordering) string {
	return makeSelectQuery(condition, ordering) + ` LIMIT 1`
}

func makeGetAllQuery(condition string, ordering q.Ordering, withCursor bool) string {
	var query string

	query = "SELECT * FROM " + tableName + " WHERE"

	if withCursor {
		if ordering == q.Ascending {
			query = query + " id > $3 AND"
		} else {
			query = query + " id < $3 AND"
		}
	} else {
		// Nonsense condition to make sure we get all records
		// TODO: This is a hack, we should use a proper way to get all records
		query = query + " (id < $3 OR id >= $3) AND "
	}

	query = query + " (" + condition + ") "
	query = query + " ORDER BY id " + q.FromOrderingWithFallback(ordering, "ASC")
	query = query + " LIMIT $2"

	return query
}

func dbGet(ctx context.Context, db *sqlx.DB, txId string, index uint32) (*model, error) {
	res := &model{}
	err := db.GetContext(ctx, res,
		makeGetQuery("transaction_id = $1 AND transaction_index = $2", q.Descending),
		txId,
		index,
	)

	return res, pgutil.CheckNoRows(err, payment.ErrNotFound)
}

func dbGetAllForTransaction(ctx context.Context, db *sqlx.DB, txId string) ([]*model, error) {
	res := []*model{}
	err := db.SelectContext(ctx, &res,
		makeSelectQuery("transaction_id = $1", q.Descending),
		txId,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetAllForAccount(ctx context.Context, db *sqlx.DB, account string, cursor uint64, limit uint, ordering q.Ordering) ([]*model, error) {
	res := []*model{}
	err := db.SelectContext(ctx, &res,
		makeGetAllQuery("source = $1 OR destination = $1", ordering, cursor > 0),
		account, limit, cursor,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetAllForAccountByType(ctx context.Context, db *sqlx.DB, account string, cursor uint64, limit uint, ordering q.Ordering, paymentType payment.PaymentType) ([]*model, error) {
	res := []*model{}

	var condition string
	if paymentType == payment.PaymentType_Send {
		condition = "source = $1"
	} else {
		condition = "destination = $1"
	}

	err := db.SelectContext(ctx, &res,
		makeGetAllQuery(condition, ordering, cursor > 0),
		account, limit, cursor,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetAllForAccountByTypeAfterBlock(ctx context.Context, db *sqlx.DB, account string, block uint64, cursor uint64, limit uint, ordering q.Ordering, paymentType payment.PaymentType) ([]*model, error) {
	res := []*model{}

	var condition string
	if paymentType == payment.PaymentType_Send {
		condition = "source = $1"
	} else {
		condition = "destination = $1"
	}

	condition += " AND block_id > $4"

	err := db.SelectContext(ctx, &res,
		makeGetAllQuery(condition, ordering, cursor > 0),
		account, limit, cursor, block,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetAllForAccountByTypeWithinBlockRange(ctx context.Context, db *sqlx.DB, account string, lowerBound, upperBound uint64, cursor uint64, limit uint, ordering q.Ordering, paymentType payment.PaymentType) ([]*model, error) {
	res := []*model{}

	var condition string
	if paymentType == payment.PaymentType_Send {
		condition = "source = $1"
	} else {
		condition = "destination = $1"
	}

	condition += " AND block_id > $4 AND block_id < $5"

	err := db.SelectContext(ctx, &res,
		makeGetAllQuery(condition, ordering, cursor > 0),
		account, limit, cursor, lowerBound, upperBound,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetAllExternalDepositsAfterBlock(ctx context.Context, db *sqlx.DB, account string, block uint64, cursor uint64, limit uint, ordering q.Ordering) ([]*model, error) {
	res := []*model{}

	condition := "destination = $1 AND block_id > $4 AND is_external"

	err := db.SelectContext(ctx, &res,
		makeGetAllQuery(condition, ordering, cursor > 0),
		account, limit, cursor, block,
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, payment.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, payment.ErrNotFound
	}

	return res, nil
}

func dbGetExternalDepositAmount(ctx context.Context, db *sqlx.DB, account string) (uint64, error) {
	var res sql.NullInt64

	query := `SELECT SUM(quantity) FROM ` + tableName + `
		WHERE destination = $1 AND is_external AND confirmation_state = 3`

	err := db.GetContext(ctx, &res, query, account)
	if err != nil {
		return 0, err
	}

	if !res.Valid {
		return 0, nil
	}
	return uint64(res.Int64), nil
}
