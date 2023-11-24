package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed rendezvous.Store
func New(db *sql.DB) rendezvous.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements rendezvous.Store.Save
func (s *store) Save(ctx context.Context, record *rendezvous.Record) error {
	obj, err := toModel(record)
	if err != nil {
		return err
	}

	err = obj.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(obj)
	res.CopyTo(record)

	return nil
}

// Get implements rendezvous.Store.Get
func (s *store) Get(ctx context.Context, key string) (*rendezvous.Record, error) {
	model, err := dbGetByKey(ctx, s.db, key)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// Delete implements rendezvous.Store.Delete
func (s *store) Delete(ctx context.Context, key string) error {
	return dbDelete(ctx, s.db, key)
}
