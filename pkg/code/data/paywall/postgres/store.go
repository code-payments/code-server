package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/paywall"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) paywall.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements paywall.Store.Put
func (s *store) Put(ctx context.Context, record *paywall.Record) error {
	m, err := toModel(record)
	if err != nil {
		return err
	}

	err = m.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(m)
	res.CopyTo(record)

	return nil
}

// GetByShortPath implements paywall.Store.GetByShortPath
func (s *store) GetByShortPath(ctx context.Context, path string) (*paywall.Record, error) {
	m, err := dbGetByShortPath(ctx, s.db, path)
	if err != nil {
		return nil, err
	}
	return fromModel(m), nil
}
