package postgres

import (
	"context"
	"database/sql"
	"time"

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

// Put implements rendezvous.Store.Put
func (s *store) Put(ctx context.Context, record *rendezvous.Record) error {
	obj, err := toModel(record)
	if err != nil {
		return err
	}

	err = obj.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(obj)
	res.CopyTo(record)

	return nil
}

// ExtendExpiry implements rendezvous.Store.ExtendExpiry
func (s *store) ExtendExpiry(ctx context.Context, key, address string, expiry time.Time) error {
	return dbExtendExpiry(ctx, s.db, key, address, expiry)
}

// Delete implements rendezvous.Store.Delete
func (s *store) Delete(ctx context.Context, key, address string) error {
	return dbDelete(ctx, s.db, key, address)
}

// Get implements rendezvous.Store.Get
func (s *store) Get(ctx context.Context, key string) (*rendezvous.Record, error) {
	model, err := dbGetByKey(ctx, s.db, key)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}
