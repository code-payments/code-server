package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/cvm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres cvm.ram.Store
func New(db *sql.DB) ram.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// InitializeMemory implements cvm.ram.Store.InitializeMemory
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

// FreeMemoryByIndex implements cvm.ram.Store.FreeMemoryByIndex
func (s *store) FreeMemoryByIndex(ctx context.Context, memoryAccount string, index uint16) error {
	return dbFreeMemoryByIndex(ctx, s.db, memoryAccount, index)
}

// FreeMemoryByAddress implements cvm.ram.Store.FreeMemoryByAddress
func (s *store) FreeMemoryByAddress(ctx context.Context, address string) error {
	return dbFreeMemoryByAddress(ctx, s.db, address)
}

// ReserveMemory implements cvm.ram.Store.ReserveMemory
func (s *store) ReserveMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType, address string) (string, uint16, error) {
	return dbReserveMemory(ctx, s.db, vm, accountType, address)
}
