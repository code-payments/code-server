package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres vm.ram.Store
func New(db *sql.DB) ram.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// InitializeMemory implements vm.ram.Store.InitializeMemory
func (s *store) InitializeMemory(ctx context.Context, record *ram.Record) error {
	model, err := toAccountModel(record)
	if err != nil {
		return err
	}

	err = model.dbInitialize(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromAccountModel(model)
	res.CopyTo(record)

	return nil
}

// FreeMemory implements vm.ram.Store.FreeMemory
func (s *store) FreeMemory(ctx context.Context, memoryAccount string, index uint16) error {
	return dbFreeMemory(ctx, s.db, memoryAccount, index)
}

// ReserveMemory implements vm.ram.Store.ReserveMemory
func (s *store) ReserveMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType) (string, uint16, error) {
	return dbReserveMemory(ctx, s.db, vm, accountType)
}
