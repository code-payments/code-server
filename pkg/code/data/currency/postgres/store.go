package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/currency"

	pg "github.com/code-payments/code-server/pkg/database/postgres"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) currency.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s store) Put(ctx context.Context, obj *currency.MultiRateRecord) error {
	return pg.ExecuteInTx(ctx, s.db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		// Loop through all rates and save individual records (within a transaction)
		for symbol, item := range obj.Rates {
			err := toModel(&currency.ExchangeRateRecord{
				Time:   obj.Time,
				Rate:   item,
				Symbol: symbol,
			}).txSave(ctx, tx)

			if err != nil {
				return pg.CheckUniqueViolation(err, currency.ErrExists)
			}
		}
		return nil
	})
}

func (s store) Get(ctx context.Context, symbol string, t time.Time) (*currency.ExchangeRateRecord, error) {
	obj, err := dbGetBySymbolAndTime(ctx, s.db, symbol, t, query.Descending)
	if err != nil {
		return nil, err
	}

	return fromModel(obj), nil
}

func (s store) GetAll(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	list, err := dbGetAllByTime(ctx, s.db, t, query.Descending)
	if err != nil {
		return nil, err
	}

	res := &currency.MultiRateRecord{
		Time:  list[0].ForTimestamp,
		Rates: map[string]float64{},
	}
	for _, item := range list {
		res.Rates[item.CurrencyCode] = item.CurrencyRate
	}

	return res, nil
}

func (s store) GetRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*currency.ExchangeRateRecord, error) {
	if interval > query.IntervalMonth {
		return nil, currency.ErrInvalidInterval
	}

	if start.IsZero() || end.IsZero() {
		return nil, currency.ErrInvalidRange
	}

	var actualStart, actualEnd time.Time
	if start.Unix() > end.Unix() {
		actualStart = end
		actualEnd = start
	} else {
		actualStart = start
		actualEnd = end
	}

	// TODO: check that the range is reasonable

	list, err := dbGetAllForRange(ctx, s.db, symbol, interval, actualStart, actualEnd, ordering)
	if err != nil {
		return nil, err
	}

	res := []*currency.ExchangeRateRecord{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}
