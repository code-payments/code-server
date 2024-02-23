package postgres

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/onramp"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed onramp.Store
func New(db *sql.DB) onramp.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements onramp.Store.Put
func (s *store) Put(ctx context.Context, record *onramp.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	if err := model.dbPut(ctx, s.db); err != nil {
		return err
	}

	res := fromModel(model)
	res.CopyTo(record)

	return nil
}

// Get implements onramp.Store.Get
func (s *store) Get(ctx context.Context, nonce uuid.UUID) (*onramp.Record, error) {
	model, err := dbGet(ctx, s.db, nonce)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
