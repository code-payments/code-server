package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/balance"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres balance.Store
func New(db *sql.DB) balance.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// SaveCheckpoint implements balance.Store.SaveCheckpoint
func (s *store) SaveCheckpoint(ctx context.Context, record *balance.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	if err := model.dbSave(ctx, s.db); err != nil {
		return err
	}

	res := fromModel(model)
	res.CopyTo(record)

	return nil
}

// GetCheckpoint implements balance.Store.GetCheckpoint
func (s *store) GetCheckpoint(ctx context.Context, account string) (*balance.Record, error) {
	model, err := dbGetCheckpoint(ctx, s.db, account)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
