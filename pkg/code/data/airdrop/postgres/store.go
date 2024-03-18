package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/airdrop"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres airdrop.Store
func New(db *sql.DB) airdrop.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// MarkIneligible implements airdrop.Store.MarkIneligible
func (s *store) MarkIneligible(ctx context.Context, owner string) error {
	model, err := toIneligibleModel(owner)
	if err != nil {
		return err
	}

	return model.dbPut(ctx, s.db)
}

// IsEligible implements airdrop.Store.IsEligible
func (s *store) IsEligible(ctx context.Context, owner string) (bool, error) {
	model, err := dbGet(ctx, s.db, owner)
	if err == errNotFound {
		// Backwards compatibility with a default true value
		return true, nil
	}

	return model.IsEligible, nil
}
