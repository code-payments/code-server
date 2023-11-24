package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/badgecount"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres badgecount.Store
func New(db *sql.DB) badgecount.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Add implements badgecount.Store.Add
func (s *store) Add(ctx context.Context, owner string, amount uint32) error {
	return dbAdd(ctx, s.db, owner, amount)
}

// Reset implements badgecount.Store.Reset
func (s *store) Reset(ctx context.Context, owner string) error {
	return dbReset(ctx, s.db, owner)
}

// Get implements badgecount.Store.Get
func (s *store) Get(ctx context.Context, owner string) (*badgecount.Record, error) {
	model, err := dbGet(ctx, s.db, owner)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
