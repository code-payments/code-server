package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
)

type store struct {
	db *sqlx.DB
}

// New returns a postgres backed user_identity.Store.
func New(db *sql.DB) user_identity.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements user_identity.Store.Put
func (s *store) Put(ctx context.Context, record *user_identity.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}
	return model.dbSave(ctx, s.db)
}

// GetByID implements user_identity.Store.GetByID
func (s *store) GetByID(ctx context.Context, id *user.UserID) (*user_identity.Record, error) {
	model, err := dbGetByID(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return fromModel(model)
}

// GetByView implements user_identity.Store.GetByView
func (s *store) GetByView(ctx context.Context, view *user.View) (*user_identity.Record, error) {
	model, err := dbGetByView(ctx, s.db, view)
	if err != nil {
		return nil, err
	}
	return fromModel(model)
}
