package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/database/query"

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

func (s *store) PutExchangeRates(ctx context.Context, obj *currency.MultiRateRecord) error {
	return pg.ExecuteInTx(ctx, s.db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		// Loop through all rates and save individual records (within a transaction)
		for symbol, item := range obj.Rates {
			err := toExchangeRateModel(&currency.ExchangeRateRecord{
				Time:   obj.Time,
				Rate:   item,
				Symbol: symbol,
			}).txSave(ctx, tx)

			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *store) GetExchangeRate(ctx context.Context, symbol string, t time.Time) (*currency.ExchangeRateRecord, error) {
	obj, err := dbGetExchangeRateBySymbolAndTime(ctx, s.db, symbol, t, query.Descending)
	if err != nil {
		return nil, err
	}

	return fromExchangeRateModel(obj), nil
}

func (s *store) GetAllExchangeRates(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	list, err := dbGetAllExchangeRatesByTime(ctx, s.db, t, query.Descending)
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

func (s *store) GetExchangeRatesInRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*currency.ExchangeRateRecord, error) {
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

	list, err := dbGetAllExchangeRatesForRange(ctx, s.db, symbol, interval, actualStart, actualEnd, ordering)
	if err != nil {
		return nil, err
	}

	res := []*currency.ExchangeRateRecord{}
	for _, item := range list {
		res = append(res, fromExchangeRateModel(item))
	}

	return res, nil
}

func (s *store) PutMetadata(ctx context.Context, record *currency.MetadataRecord) error {
	model, err := toMetadataModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	fromMetadataModel(model).CopyTo(record)

	return nil
}

func (s *store) GetMetadata(ctx context.Context, mint string) (*currency.MetadataRecord, error) {
	model, err := dbGetMetadataByMint(ctx, s.db, mint)
	if err != nil {
		return nil, err
	}
	return fromMetadataModel(model), nil
}

func (s *store) PutReserveRecord(ctx context.Context, record *currency.ReserveRecord) error {
	model, err := toReserveModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	fromReserveModel(model).CopyTo(record)

	return nil
}

func (s *store) GetReserveAtTime(ctx context.Context, mint string, t time.Time) (*currency.ReserveRecord, error) {
	model, err := dbGetReserveByMintAndTime(ctx, s.db, mint, t, query.Descending)
	if err != nil {
		return nil, err
	}
	return fromReserveModel(model), nil
}
