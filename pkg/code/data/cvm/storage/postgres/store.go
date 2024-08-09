package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres vm.storage.Store
func New(db *sql.DB) storage.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// InitializeStorage implements vm.storage.Store.InitializeStorage
func (s *store) InitializeStorage(ctx context.Context, record *storage.Record) error {
	if record.AvailableCapacity != storage.GetMaxCapacity(record.Levels) {
		return storage.ErrInvalidInitialCapacity
	}

	model, err := toAccountModel(record)
	if err != nil {
		return err
	}

	err = model.dbInitialize(ctx, s.db)
	if err != nil {
		return err
	}

	fromAccountModel(model).CopyTo(record)

	return nil
}

// FindAnyWithAvailableCapacity implements cvm.storage.Store.FindAnyWithAvailableCapacity
func (s *store) FindAnyWithAvailableCapacity(ctx context.Context, vm string, purpose storage.Purpose, minCapacity uint64) (*storage.Record, error) {
	model, err := dbFindAnyWithAvailableCapacity(ctx, s.db, vm, purpose, minCapacity)
	if err != nil {
		return nil, err
	}
	return fromAccountModel(model), nil
}

// ReserveStorage implements cvm.storage.Store.ReserveStorage
func (s *store) ReserveStorage(ctx context.Context, vm string, purpose storage.Purpose, address string) (string, error) {
	return dbReserveStorage(ctx, s.db, vm, purpose, address)
}
