package postgres

import (
	"context"
	"database/sql"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres twitter.Store
func New(db *sql.DB) twitter.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements twitter.Store.Save
func (s *store) Save(ctx context.Context, record *twitter.Record) error {
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

// Get implements twitter.Store.Get
func (s *store) Get(ctx context.Context, username string) (*twitter.Record, error) {
	model, err := dbGet(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
