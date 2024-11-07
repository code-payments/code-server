package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/cvm/ram"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

const (
	accountTableName         = "codewallet__core_vmmemoryaccount"
	allocatedMemoryTableName = "codewallet__core_vmmemoryallocatedmemory"
)

type accountModel struct {
	Id sql.NullInt64 `db:"id"`

	Vm string `db:"vm"`

	Address string `db:"address"`

	Capacity   uint16 `db:"capacity"`
	NumSectors uint16 `db:"num_sectors"`
	NumPages   uint16 `db:"num_pages"`
	PageSize   uint8  `db:"page_size"`

	StoredAccountType uint8 `db:"stored_account_type"`

	CreatedAt time.Time `db:"created_at"`
}

type allocatedMemoryModel struct {
	Id sql.NullInt64 `db:"id"`

	Vm string `db:"vm"`

	MemoryAccount     string         `db:"memory_account"`
	Index             uint16         `db:"index"`
	IsAllocated       bool           `db:"is_allocated"`
	StoredAccountType uint8          `db:"stored_account_type"`
	Address           sql.NullString `db:"address"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toAccountModel(obj *ram.Record) (*accountModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &accountModel{
		Vm: obj.Vm,

		Address: obj.Address,

		Capacity:   obj.Capacity,
		NumSectors: obj.NumSectors,
		NumPages:   obj.NumPages,
		PageSize:   obj.PageSize,

		StoredAccountType: uint8(obj.StoredAccountType),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromAccountModel(obj *accountModel) *ram.Record {
	return &ram.Record{
		Id: uint64(obj.Id.Int64),

		Vm: obj.Vm,

		Address: obj.Address,

		Capacity:   obj.Capacity,
		NumSectors: obj.NumSectors,
		NumPages:   obj.NumPages,
		PageSize:   obj.PageSize,

		StoredAccountType: cvm.VirtualAccountType(obj.StoredAccountType),

		CreatedAt: obj.CreatedAt,
	}
}

func (m *accountModel) dbInitialize(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query1 := `INSERT INTO ` + accountTableName + `
				(vm, address, capacity, num_sectors, num_pages, page_size, stored_account_type, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				RETURNING
					id, vm, address, capacity, num_sectors, num_pages, page_size, stored_account_type, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		err := tx.QueryRowxContext(
			ctx,
			query1,
			m.Vm,
			m.Address,
			m.Capacity,
			m.NumSectors,
			m.NumPages,
			m.PageSize,
			m.StoredAccountType,
			m.CreatedAt,
		).StructScan(m)

		if err != nil {
			return pgutil.CheckUniqueViolation(err, ram.ErrAlreadyInitialized)
		}

		query2 := `INSERT INTO ` + allocatedMemoryTableName + `
				(vm, memory_account, index, is_allocated, stored_account_type, last_updated_at)
				SELECT $1, $2, generate_series(0, $3), $4, $5, $6`

		_, err = tx.ExecContext(
			ctx,
			query2,
			m.Vm,
			m.Address,
			ram.GetActualCapcity(fromAccountModel(m))-1,
			false,
			m.StoredAccountType,
			m.CreatedAt,
		)
		return err
	})
}

func dbFreeMemoryByIndex(ctx context.Context, db *sqlx.DB, memoryAccount string, index uint16) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		var model allocatedMemoryModel

		query := `UPDATE ` + allocatedMemoryTableName + `
			SET is_allocated = false, address = NULL, last_updated_at = $3
			WHERE memory_account = $1 and index = $2 AND is_allocated
			RETURNING id, vm, memory_account, index, is_allocated, stored_account_type, address, last_updated_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			memoryAccount,
			index,
			time.Now(),
		).StructScan(&model)

		return pgutil.CheckNoRows(err, ram.ErrNotReserved)
	})
}

func dbFreeMemoryByAddress(ctx context.Context, db *sqlx.DB, address string) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		var model allocatedMemoryModel

		query := `UPDATE ` + allocatedMemoryTableName + `
			SET is_allocated = false, address = NULL, last_updated_at = $2
			WHERE address = $1 AND is_allocated
			RETURNING id, vm, memory_account, index, is_allocated, stored_account_type, address, last_updated_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			address,
			time.Now(),
		).StructScan(&model)

		return pgutil.CheckNoRows(err, ram.ErrNotReserved)
	})
}

func dbReserveMemory(ctx context.Context, db *sqlx.DB, vm string, accountType cvm.VirtualAccountType, address string) (string, uint16, error) {
	var memoryAccount string
	var index uint16
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		var model allocatedMemoryModel

		query := `UPDATE ` + allocatedMemoryTableName + `
			SET is_allocated = true, address = $3, last_updated_at = $4
			WHERE id IN (
				SELECT id FROM ` + allocatedMemoryTableName + `
				WHERE vm = $1 AND stored_account_type = $2 AND NOT is_allocated
				LIMIT 1
				FOR UPDATE
			)
			RETURNING id, vm, memory_account, index, is_allocated, stored_account_type, address, last_updated_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			vm,
			accountType,
			address,
			time.Now(),
		).StructScan(&model)
		if err != nil {
			return pgutil.CheckUniqueViolation(pgutil.CheckNoRows(err, ram.ErrNoFreeMemory), ram.ErrAddressAlreadyReserved)
		}

		memoryAccount = model.MemoryAccount
		index = model.Index

		return nil
	})
	return memoryAccount, index, err
}
