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

// SaveExternalCheckpoint implements balance.Store.SaveExternalCheckpoint
func (s *store) SaveExternalCheckpoint(ctx context.Context, record *balance.ExternalCheckpointRecord) error {
	model, err := toExternalCheckpointModel(record)
	if err != nil {
		return err
	}

	if err := model.dbSave(ctx, s.db); err != nil {
		return err
	}

	res := fromExternalCheckpoingModel(model)
	res.CopyTo(record)

	return nil
}

// GetExternalCheckpoint implements balance.Store.GetExternalCheckpoint
func (s *store) GetExternalCheckpoint(ctx context.Context, account string) (*balance.ExternalCheckpointRecord, error) {
	model, err := dbGetExternalCheckpoint(ctx, s.db, account)
	if err != nil {
		return nil, err
	}
	return fromExternalCheckpoingModel(model), nil
}
