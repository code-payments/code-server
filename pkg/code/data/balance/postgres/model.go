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
	externalCheckpointTableName = "codewallet__core_externalbalancecheckpoint"
)

type externalCheckpointModel struct {
	Id sql.NullInt64 `db:"id"`

	TokenAccount   string `db:"token_account"`
	Quarks         uint64 `db:"quarks"`
	SlotCheckpoint uint64 `db:"slot_checkpoint"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toExternalCheckpointModel(obj *balance.ExternalCheckpointRecord) (*externalCheckpointModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &externalCheckpointModel{
		TokenAccount:   obj.TokenAccount,
		Quarks:         obj.Quarks,
		SlotCheckpoint: obj.SlotCheckpoint,
		LastUpdatedAt:  obj.LastUpdatedAt,
	}, nil
}

func fromExternalCheckpoingModel(obj *externalCheckpointModel) *balance.ExternalCheckpointRecord {
	return &balance.ExternalCheckpointRecord{
		Id:             uint64(obj.Id.Int64),
		TokenAccount:   obj.TokenAccount,
		Quarks:         obj.Quarks,
		SlotCheckpoint: obj.SlotCheckpoint,
		LastUpdatedAt:  obj.LastUpdatedAt,
	}
}

func (m *externalCheckpointModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + externalCheckpointTableName + `
			(token_account, quarks, slot_checkpoint, last_updated_at)
			VALUES ($1, $2, $3, $4)

			ON CONFLICT (token_account)
			DO UPDATE
				SET quarks = $2, slot_checkpoint = $3, last_updated_at = $4
				WHERE ` + externalCheckpointTableName + `.token_account = $1 AND ` + externalCheckpointTableName + `.slot_checkpoint < $3

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

func dbGetExternalCheckpoint(ctx context.Context, db *sqlx.DB, account string) (*externalCheckpointModel, error) {
	res := &externalCheckpointModel{}

	query := `SELECT
		id, token_account, quarks, slot_checkpoint, last_updated_at
		FROM ` + externalCheckpointTableName + `
		WHERE token_account = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, account)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, balance.ErrCheckpointNotFound)
	}
	return res, nil
}
