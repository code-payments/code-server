package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/event"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed rendezvous.Store
func New(db *sql.DB) event.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements event.Store.Save
func (s *store) Save(ctx context.Context, record *event.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	fromModel(model).CopyTo(record)
	return nil
}

// Get implements event.Store.Get
func (s *store) Get(ctx context.Context, id string) (*event.Record, error) {
	model, err := dbGet(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
