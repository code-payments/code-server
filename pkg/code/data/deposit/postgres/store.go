package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/deposit"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres deposit.Store
func New(db *sql.DB) deposit.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements deposit.Store.Save
func (s *store) Save(ctx context.Context, record *deposit.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(model)
	res.CopyTo(record)

	return nil
}

// Get implements deposit.Store.Get
func (s *store) Get(ctx context.Context, signature, account string) (*deposit.Record, error) {
	model, err := dbGet(ctx, s.db, signature, account)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetQuarkAmount implements deposit.Store.GetQuarkAmount
func (s *store) GetQuarkAmount(ctx context.Context, account string) (uint64, error) {
	return dbGetQuarkAmount(ctx, s.db, account)
}

// GetQuarkAmountBatch implements deposit.Store.GetQuarkAmountBatch
func (s *store) GetQuarkAmountBatch(ctx context.Context, accounts ...string) (map[string]uint64, error) {
	return dbGetQuarkAmountBatch(ctx, s.db, accounts...)
}

// GetUsdAmount implements deposit.Store.GetUsdAmount
func (s *store) GetUsdAmount(ctx context.Context, account string) (float64, error) {
	return dbGetUsdAmount(ctx, s.db, account)
}
