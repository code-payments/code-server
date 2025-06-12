package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/balance"
	pg "github.com/code-payments/code-server/pkg/database/postgres"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	cachedBalanceVersionTableName = "codewallet__core_cachedbalanceversion"
	externalCheckpointTableName   = "codewallet__core_externalbalancecheckpoint"
)

type externalCheckpointModel struct {
	Id sql.NullInt64 `db:"id"`

	TokenAccount   string `db:"token_account"`
	Quarks         uint64 `db:"quarks"`
	SlotCheckpoint uint64 `db:"slot_checkpoint"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func dbGetCachedVersion(ctx context.Context, db *sqlx.DB, account string) (uint64, error) {
	var res uint64
	query := `SELECT version FROM ` + cachedBalanceVersionTableName + `
		WHERE token_account = $1`
	err := db.GetContext(ctx, &res, query, account)
	if pg.IsNoRows(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return res, nil
}

func dbAdvanceCachedVersion(ctx context.Context, db *sqlx.DB, account string, currentVersion uint64) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + cachedBalanceVersionTableName + `
			(token_account, version)
			VALUES ($1, 1)
			RETURNING version
		`
		params := []any{account}
		if currentVersion > 0 {
			query = `UPDATE ` + cachedBalanceVersionTableName + `
				SET version = version + 1
				WHERE token_account = $1 AND version = $2
				RETURNING version
			`
			params = append(params, currentVersion)
		}

		var res uint64
		err := tx.GetContext(ctx, &res, query, params...)
		if pg.IsNoRows(err) || pg.IsUniqueViolation(err) {
			return balance.ErrStaleCachedBalanceVersion
		}
		if err != nil {
			return err
		}
		return nil
	})

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

	query := `SELECT id, token_account, quarks, slot_checkpoint, last_updated_at FROM ` + externalCheckpointTableName + `
		WHERE token_account = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, account)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, balance.ErrCheckpointNotFound)
	}
	return res, nil
}
