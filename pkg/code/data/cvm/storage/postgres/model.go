package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	accountTableName = "codewallet__core_vmstorageaccount"
)

type accountModel struct {
	Id sql.NullInt64 `db:"id"`

	Vm string `db"vm"`

	Name              string `db:"name"`
	Address           string `db:"address"`
	Levels            uint8  `db:"levels"`
	AvailableCapacity uint64 `db:"available_capacity"`
	Purpose           uint8  `db:"purpose"`

	CreatedAt time.Time `db:"created_at"`
}

func toAccountModel(obj *storage.Record) (*accountModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &accountModel{
		Vm: obj.Vm,

		Name:              obj.Name,
		Address:           obj.Address,
		Levels:            obj.Levels,
		AvailableCapacity: obj.AvailableCapacity,
		Purpose:           uint8(obj.Purpose),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromAccountModel(obj *accountModel) *storage.Record {
	return &storage.Record{
		Id: uint64(obj.Id.Int64),

		Vm: obj.Vm,

		Name:              obj.Name,
		Address:           obj.Address,
		Levels:            obj.Levels,
		AvailableCapacity: obj.AvailableCapacity,
		Purpose:           storage.Purpose(obj.Purpose),

		CreatedAt: obj.CreatedAt,
	}
}

func (m *accountModel) dbInitialize(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + accountTableName + `
				(vm, name, address, levels, available_capacity, purpose, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				RETURNING
					id, vm, name, address, levels, available_capacity, purpose, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Vm,
			m.Name,
			m.Address,
			m.Levels,
			m.AvailableCapacity,
			m.Purpose,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, storage.ErrAlreadyInitialized)
	})
}

func dbFindAnyWithAvailableCapacity(ctx context.Context, db *sqlx.DB, vm string, purpose storage.Purpose, minCapacity uint64) (*accountModel, error) {
	res := &accountModel{}

	query := `SELECT
		id, vm, name, address, levels, available_capacity, purpose, created_at
		FROM ` + accountTableName + `
		WHERE vm = $1 AND purpose = $2 AND available_capacity >= $3
		LIMIT 1`

	err := db.GetContext(
		ctx,
		res,
		query,
		vm,
		purpose,
		minCapacity,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, storage.ErrNotFound)
	}
	return res, nil
}
