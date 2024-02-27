package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	db *sqlx.DB
}

// New returns a new in postgres preferences.Store
func New(db *sql.DB) preferences.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save saves a preferences record
func (s *store) Save(ctx context.Context, record *preferences.Record) error {
	m, err := toModel(record)
	if err != nil {
		return err
	}

	err = m.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res, err := fromModel(m)
	if err != nil {
		return err
	}
	res.CopyTo(record)

	return nil
}

// Get gets a a preference record by a data container
func (s *store) Get(ctx context.Context, id *user.DataContainerID) (*preferences.Record, error) {
	m, err := dbGet(ctx, s.db, id)
	if err != nil {
		return nil, err
	}

	return fromModel(m)
}
