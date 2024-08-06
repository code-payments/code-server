package postgres

import (
	"context"
	"database/sql"
	"errors"

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
	return errors.New("not implemented")
}

// FreeMemory implements vm.ram.Store.FreeMemory
func (s *store) FreeMemory(ctx context.Context, memoryAccount string, index uint16) error {
	return errors.New("not implemented")
}

// ReserveMemory implements vm.ram.Store.ReserveMemory
func (s *store) ReserveMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType) (string, uint16, error) {
	return "", 0, errors.New("not implemetned")
}
