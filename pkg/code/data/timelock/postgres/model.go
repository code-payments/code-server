package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/timelock"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	tableName = "codewallet__core_timelock"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Address string `db:"address"`
	Bump    uint   `db:"bump"`

	VaultAddress string `db:"vault_address"`
	VaultBump    uint   `db:"vault_bump"`
	VaultOwner   string `db:"vault_owner"`
	VaultState   uint   `db:"vault_state"`

	DepositPdaAddress string `db:"deposit_pda_address"`
	DepositPdaBump    uint   `db:"deposit_pda_bump"`

	UnlockAt sql.NullInt64 `db:"unlock_at"`

	Block uint64 `db:"block"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toModel(obj *timelock.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	var unlockAt sql.NullInt64
	if obj.UnlockAt != nil {
		unlockAt.Valid = true
		unlockAt.Int64 = int64(*obj.UnlockAt)
	}

	return &model{
		Address: obj.Address,
		Bump:    uint(obj.Bump),

		VaultAddress: obj.VaultAddress,
		VaultBump:    uint(obj.VaultBump),
		VaultOwner:   obj.VaultOwner,
		VaultState:   uint(obj.VaultState),

		DepositPdaAddress: obj.DepositPdaAddress,
		DepositPdaBump:    uint(obj.DepositPdaBump),

		UnlockAt: unlockAt,

		Block: obj.Block,

		LastUpdatedAt: obj.LastUpdatedAt,
	}, nil
}

func fromModel(obj *model) *timelock.Record {
	var unlockAt *uint64
	if obj.UnlockAt.Valid {
		value := uint64(obj.UnlockAt.Int64)
		unlockAt = &value
	}

	return &timelock.Record{
		Id: uint64(obj.Id.Int64),

		Address: obj.Address,
		Bump:    uint8(obj.Bump),

		VaultAddress: obj.VaultAddress,
		VaultBump:    uint8(obj.VaultBump),
		VaultOwner:   obj.VaultOwner,
		VaultState:   timelock_token.TimelockState(obj.VaultState),

		DepositPdaAddress: obj.DepositPdaAddress,
		DepositPdaBump:    uint8(obj.DepositPdaBump),

		UnlockAt: unlockAt,

		Block: obj.Block,

		LastUpdatedAt: obj.LastUpdatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)

			ON CONFLICT (address)
			DO UPDATE
				SET vault_state = $6, unlock_at = $9, block = $10, last_updated_at = $11
				WHERE ` + tableName + `.address = $1 AND ` + tableName + `.vault_address = $3 AND ` + tableName + `.block < $10

			RETURNING
				id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at`

		m.LastUpdatedAt = time.Now()

		err := tx.QueryRowxContext(
			ctx,
			query,

			m.Address,
			m.Bump,

			m.VaultAddress,
			m.VaultBump,
			m.VaultOwner,
			m.VaultState,

			m.DepositPdaAddress,
			m.DepositPdaBump,

			m.UnlockAt,

			m.Block,

			m.LastUpdatedAt.UTC(),
		).StructScan(m)

		return pgutil.CheckNoRows(err, timelock.ErrStaleTimelockState)
	})
}

func dbGetByAddress(ctx context.Context, db *sqlx.DB, address string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at
		FROM ` + tableName + `
		WHERE address = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, timelock.ErrTimelockNotFound)
	}
	return res, nil
}

func dbGetByVault(ctx context.Context, db *sqlx.DB, vault string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at
		FROM ` + tableName + `
		WHERE vault_address = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, vault)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, timelock.ErrTimelockNotFound)
	}
	return res, nil
}

func dbGetByVaultBatch(ctx context.Context, db *sqlx.DB, vaults ...string) ([]*model, error) {
	res := []*model{}

	individualFilters := make([]string, len(vaults))
	for i, vault := range vaults {
		individualFilters[i] = fmt.Sprintf("'%s'", vault)
	}

	query := fmt.Sprintf(
		`SELECT id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at
		FROM `+tableName+`
		WHERE vault_address IN (%s)`,
		strings.Join(individualFilters, ", "),
	)

	err := db.SelectContext(ctx, &res, query)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, timelock.ErrTimelockNotFound)
	}
	if len(res) != len(vaults) {
		return nil, timelock.ErrTimelockNotFound
	}
	return res, nil
}

func dbGetByDepositPda(ctx context.Context, db *sqlx.DB, depositPda string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at
		FROM ` + tableName + `
		WHERE deposit_pda_address = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, depositPda)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, timelock.ErrTimelockNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state timelock_token.TimelockState, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*model, error) {
	res := []*model{}

	query := `SELECT
		id, address, bump, vault_address, vault_bump, vault_owner, vault_state, deposit_pda_address, deposit_pda_bump, unlock_at, block, last_updated_at
		FROM ` + tableName + `
		WHERE (vault_state = $1)
	`

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, timelock.ErrTimelockNotFound)
	}

	if len(res) == 0 {
		return nil, timelock.ErrTimelockNotFound
	}
	return res, nil
}

func dbGetCountByState(ctx context.Context, db *sqlx.DB, state timelock_token.TimelockState) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + ` WHERE vault_state = $1`
	err := db.GetContext(ctx, &res, query, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}
