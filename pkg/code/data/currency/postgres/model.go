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
	tableName  = "codewallet__core_exchangerate"
	dateFormat = "2006-01-02"
)

type model struct {
	Id           sql.NullInt64 `db:"id"`
	ForDate      string        `db:"for_date"`
	ForTimestamp time.Time     `db:"for_timestamp"`
	CurrencyCode string        `db:"currency_code"`
	CurrencyRate float64       `db:"currency_rate"`
}

func toModel(obj *currency.ExchangeRateRecord) *model {
	return &model{
		Id:           sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		ForDate:      obj.Time.UTC().Format(dateFormat),
		ForTimestamp: obj.Time.UTC(),
		CurrencyCode: obj.Symbol,
		CurrencyRate: obj.Rate,
	}
}

func fromModel(obj *model) *currency.ExchangeRateRecord {
	return &currency.ExchangeRateRecord{
		Id:     uint64(obj.Id.Int64),
		Time:   obj.ForTimestamp.UTC(),
		Symbol: obj.CurrencyCode,
		Rate:   obj.CurrencyRate,
	}
}

func makeInsertQuery() string {
	return `INSERT INTO ` + tableName + ` (for_date, for_timestamp, currency_code, currency_rate)
		VALUES ($1, $2, $3, $4) RETURNING *;`
}

func makeSelectQuery(condition string, ordering q.Ordering) string {
	return `SELECT * FROM ` + tableName + ` WHERE ` + condition + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
}

func makeGetQuery(condition string, ordering q.Ordering) string {
	return makeSelectQuery(condition, ordering) + ` LIMIT 1`
}

func makeRangeQuery(condition string, ordering q.Ordering, interval q.Interval) string {
	var query, bucket string

	if interval == q.IntervalRaw {
		query = `SELECT *`
	} else {
		bucket = `date_trunc('` + q.FromIntervalWithFallback(interval, "hour") + `', for_timestamp)`
		query = `SELECT DISTINCT ON (` + bucket + `) *`
	}

	query = query + ` FROM ` + tableName + ` WHERE ` + condition

	if interval == q.IntervalRaw {
		query = query + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
	} else {
		query = query + ` ORDER BY ` + bucket + `, for_timestamp DESC` // keep only the latest record for each bucket
	}

	//fmt.Printf("query: %s\n", query)

	return query
}

func (m *model) txSave(ctx context.Context, tx *sqlx.Tx) error {
	err := tx.QueryRowxContext(ctx,
		makeInsertQuery(),
		m.ForDate,
		m.ForTimestamp,
		m.CurrencyCode,
		m.CurrencyRate,
	).StructScan(m)

	return pgutil.CheckUniqueViolation(err, currency.ErrExists)
}

func dbGetBySymbolAndTime(ctx context.Context, db *sqlx.DB, symbol string, t time.Time, ordering q.Ordering) (*model, error) {
	res := &model{}
	err := db.GetContext(ctx, res,
		makeGetQuery("currency_code = $1 AND for_date = $2 AND for_timestamp <= $3", ordering),
		symbol,
		t.UTC().Format(dateFormat),
		t.UTC(),
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}

func dbGetAllByTime(ctx context.Context, db *sqlx.DB, t time.Time, ordering q.Ordering) ([]*model, error) {
	query := `SELECT DISTINCT ON (currency_code) *
		FROM ` + tableName + `
		WHERE for_date = $1 AND for_timestamp <= $2
		ORDER BY currency_code, for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")

	res := []*model{}
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

func dbGetAllForRange(ctx context.Context, db *sqlx.DB, symbol string, interval q.Interval, start time.Time, end time.Time, ordering q.Ordering) ([]*model, error) {
	res := []*model{}
	err := db.SelectContext(ctx, &res,
		makeRangeQuery("currency_code = $1 AND for_timestamp >= $2 AND for_timestamp <= $3", ordering, interval),
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
