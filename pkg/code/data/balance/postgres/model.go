package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/balance"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_balancecheckpoint"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	TokenAccount   string `db:"token_account"`
	Quarks         uint64 `db:"quarks"`
	SlotCheckpoint uint64 `db:"slot_checkpoint"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toModel(obj *balance.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		TokenAccount:   obj.TokenAccount,
		Quarks:         obj.Quarks,
		SlotCheckpoint: obj.SlotCheckpoint,
		LastUpdatedAt:  obj.LastUpdatedAt,
	}, nil
}

func fromModel(obj *model) *balance.Record {
	return &balance.Record{
		Id:             uint64(obj.Id.Int64),
		TokenAccount:   obj.TokenAccount,
		Quarks:         obj.Quarks,
		SlotCheckpoint: obj.SlotCheckpoint,
		LastUpdatedAt:  obj.LastUpdatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(token_account, quarks, slot_checkpoint, last_updated_at)
			VALUES ($1, $2, $3, $4)

			ON CONFLICT (token_account)
			DO UPDATE
				SET quarks = $2, slot_checkpoint = $3, last_updated_at = $4
				WHERE ` + tableName + `.token_account = $1 AND ` + tableName + `.slot_checkpoint < $3

			RETURNING
				id, token_account, quarks, slot_checkpoint, last_updated_at`

		m.LastUpdatedAt = time.Now()

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.TokenAccount,
			m.Quarks,
			m.SlotCheckpoint,
			m.LastUpdatedAt.UTC(),
		).StructScan(m)

		return pgutil.CheckNoRows(err, balance.ErrStaleCheckpoint)
	})
}

func dbGetCheckpoint(ctx context.Context, db *sqlx.DB, account string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, token_account, quarks, slot_checkpoint, last_updated_at
		FROM ` + tableName + `
		WHERE token_account = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, account)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, balance.ErrCheckpointNotFound)
	}
	return res, nil
}
