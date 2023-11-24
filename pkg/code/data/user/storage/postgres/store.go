package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"
)

type store struct {
	db *sqlx.DB
}

// New returns a postgres backed user_storage.Store.
func New(db *sql.DB) user_storage.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements user_storage.Store.Put
func (s *store) Put(ctx context.Context, container *user_storage.Record) error {
	model, err := toModel(container)
	if err != nil {
		return err
	}
	return model.dbSave(ctx, s.db)
}

// GetByID implements user_storage.Store.GetByID
func (s *store) GetByID(ctx context.Context, id *user.DataContainerID) (*user_storage.Record, error) {
	model, err := dbGetByID(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return fromModel(model)
}

// GeByFeatures implements user_storage.Store.GetByFeatures
func (s *store) GetByFeatures(ctx context.Context, ownerAccount string, features *user.IdentifyingFeatures) (*user_storage.Record, error) {
	model, err := dbGetByFeatures(ctx, s.db, ownerAccount, features)
	if err != nil {
		return nil, err
	}
	return fromModel(model)
}
