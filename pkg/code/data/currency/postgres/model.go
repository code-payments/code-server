package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	q "github.com/code-payments/code-server/pkg/database/query"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	exchangeRateTableName = "codewallet__core_exchangerate"
	reserveTableName      = "codewallet__core_currencyreserve"
	dateFormat            = "2006-01-02"
)

type exchangeRateModel struct {
	Id           sql.NullInt64 `db:"id"`
	ForDate      string        `db:"for_date"`
	ForTimestamp time.Time     `db:"for_timestamp"`
	CurrencyCode string        `db:"currency_code"`
	CurrencyRate float64       `db:"currency_rate"`
}

func toExchangeRateModel(obj *currency.ExchangeRateRecord) *exchangeRateModel {
	return &exchangeRateModel{
		Id:           sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		ForDate:      obj.Time.UTC().Format(dateFormat),
		ForTimestamp: obj.Time.UTC(),
		CurrencyCode: obj.Symbol,
		CurrencyRate: obj.Rate,
	}
}

func fromExchangeRateModel(obj *exchangeRateModel) *currency.ExchangeRateRecord {
	return &currency.ExchangeRateRecord{
		Id:     uint64(obj.Id.Int64),
		Time:   obj.ForTimestamp.UTC(),
		Symbol: obj.CurrencyCode,
		Rate:   obj.CurrencyRate,
	}
}

type reserveModel struct {
	Id                sql.NullInt64 `db:"id"`
	ForDate           string        `db:"for_date"`
	ForTimestamp      time.Time     `db:"for_timestamp"`
	Mint              string        `db:"mint"`
	SupplyFromBonding uint64        `db:"supply_from_bonding"`
	CoreMintLocked    uint64        `db:"core_mint_locked"`
}

func toReserveModel(obj *currency.ReserveRecord) *reserveModel {
	return &reserveModel{
		Id:                sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		ForDate:           obj.Time.UTC().Format(dateFormat),
		ForTimestamp:      obj.Time.UTC(),
		Mint:              obj.Mint,
		SupplyFromBonding: obj.SupplyFromBonding,
		CoreMintLocked:    obj.CoreMintLocked,
	}
}

func fromReserveModel(obj *reserveModel) *currency.ReserveRecord {
	return &currency.ReserveRecord{
		Id:                uint64(obj.Id.Int64),
		Time:              obj.ForTimestamp.UTC(),
		Mint:              obj.Mint,
		SupplyFromBonding: obj.SupplyFromBonding,
		CoreMintLocked:    obj.CoreMintLocked,
	}
}

func makeSelectQuery(table, condition string, ordering q.Ordering) string {
	return `SELECT * FROM ` + table + ` WHERE ` + condition + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
}

func makeGetQuery(table, condition string, ordering q.Ordering) string {
	return makeSelectQuery(table, condition, ordering) + ` LIMIT 1`
}

func makeRangeQuery(table, condition string, ordering q.Ordering, interval q.Interval) string {
	var query, bucket string

	if interval == q.IntervalRaw {
		query = `SELECT *`
	} else {
		bucket = `date_trunc('` + q.FromIntervalWithFallback(interval, "hour") + `', for_timestamp)`
		query = `SELECT DISTINCT ON (` + bucket + `) *`
	}

	query = query + ` FROM ` + table + ` WHERE ` + condition

	if interval == q.IntervalRaw {
		query = query + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
	} else {
		query = query + ` ORDER BY ` + bucket + `, for_timestamp DESC` // keep only the latest record for each bucket
	}

	return query
}

func (m *exchangeRateModel) txSave(ctx context.Context, tx *sqlx.Tx) error {
	err := tx.QueryRowxContext(ctx,
		`INSERT INTO `+exchangeRateTableName+` (for_date, for_timestamp, currency_code, currency_rate)
		VALUES ($1, $2, $3, $4) RETURNING *;`,
		m.ForDate,
		m.ForTimestamp,
		m.CurrencyCode,
		m.CurrencyRate,
	).StructScan(m)

	return pgutil.CheckUniqueViolation(err, currency.ErrExists)
}

func (m *reserveModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		err := tx.QueryRowxContext(ctx,
			`INSERT INTO `+reserveTableName+` (for_date, for_timestamp, mint, supply_from_bonding, core_mint_locked)
			VALUES ($1, $2, $3, $4, $5) RETURNING *;`,
			m.ForDate,
			m.ForTimestamp,
			m.Mint,
			m.SupplyFromBonding,
			m.CoreMintLocked,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, currency.ErrExists)
	})
}

func dbGetExchangeRateBySymbolAndTime(ctx context.Context, db *sqlx.DB, symbol string, t time.Time, ordering q.Ordering) (*exchangeRateModel, error) {
	res := &exchangeRateModel{}
	err := db.GetContext(ctx, res,
		makeGetQuery(exchangeRateTableName, "currency_code = $1 AND for_date = $2 AND for_timestamp <= $3", ordering),
		symbol,
		t.UTC().Format(dateFormat),
		t.UTC(),
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}

func dbGetAllExchangeRatesByTime(ctx context.Context, db *sqlx.DB, t time.Time, ordering q.Ordering) ([]*exchangeRateModel, error) {
	query := `SELECT DISTINCT ON (currency_code) *
		FROM ` + exchangeRateTableName + `
		WHERE for_date = $1 AND for_timestamp <= $2
		ORDER BY currency_code, for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")

	res := []*exchangeRateModel{}
	err := db.SelectContext(ctx, &res, query, t.UTC().Format(dateFormat), t.UTC())

	if err != nil {
		return nil, pgutil.CheckNoRows(err, currency.ErrNotFound)
	}
	if res == nil {
		return nil, currency.ErrNotFound
	}
	if len(res) == 0 {
		return nil, currency.ErrNotFound
	}

	return res, nil
}

func dbGetAllExchangeRatesForRange(ctx context.Context, db *sqlx.DB, symbol string, interval q.Interval, start time.Time, end time.Time, ordering q.Ordering) ([]*exchangeRateModel, error) {
	res := []*exchangeRateModel{}
	err := db.SelectContext(ctx, &res,
		makeRangeQuery(exchangeRateTableName, "currency_code = $1 AND for_timestamp >= $2 AND for_timestamp <= $3", ordering, interval),
		symbol, start.UTC(), end.UTC(),
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, currency.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, currency.ErrNotFound
	}

	return res, nil
}

func dbGetReserveByMintAndTime(ctx context.Context, db *sqlx.DB, mint string, t time.Time, ordering q.Ordering) (*reserveModel, error) {
	res := &reserveModel{}
	err := db.GetContext(ctx, res,
		makeGetQuery(reserveTableName, "mint = $1 AND for_date = $2 AND for_timestamp <= $3", ordering),
		mint,
		t.UTC().Format(dateFormat),
		t.UTC(),
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}
