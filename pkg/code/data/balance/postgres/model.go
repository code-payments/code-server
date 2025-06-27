package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/balance"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	cachedBalanceVersionTableName = "codewallet__core_cachedbalanceversion"
	openCloseLocksTableName       = "codewallet__core_opencloselocks"
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
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		insertQuery := `INSERT INTO ` + cachedBalanceVersionTableName + `
			(token_account, version)
			VALUES($1, 0)
			ON CONFLICT DO NOTHING
		`
		sqlResult, err := tx.ExecContext(ctx, insertQuery, account)
		if err != nil {
			return err
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 1 {
			res = 0
			return nil
		}

		selectQuery := `SELECT version FROM ` + cachedBalanceVersionTableName + `
			WHERE token_account = $1
			FOR UPDATE`
		return db.GetContext(ctx, &res, selectQuery, account)
	})
	return res, err

}

func dbAdvanceCachedVersion(ctx context.Context, db *sqlx.DB, account string, currentVersion uint64) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		var res uint64
		query := `UPDATE ` + cachedBalanceVersionTableName + `
			SET version = version + 1
			WHERE token_account = $1 AND version = $2
			RETURNING version
		`
		err := tx.GetContext(ctx, &res, query, account, currentVersion)
		if pgutil.IsNoRows(err) || pgutil.IsUniqueViolation(err) {
			return balance.ErrStaleCachedBalanceVersion
		}
		if err != nil {
			return err
		}
		return nil
	})

}

func dbCheckNotClosed(ctx context.Context, db *sqlx.DB, account string) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		insertQuery := `INSERT INTO ` + openCloseLocksTableName + `
			(token_account, is_open)
			VALUES ($1, TRUE)
			ON CONFLICT DO NOTHING
		`

		_, err := tx.ExecContext(ctx, insertQuery, account)
		if err != nil {
			return err
		}

		selectQuery := `SELECT is_open FROM ` + openCloseLocksTableName + `
			WHERE token_account = $1
			FOR UPDATE
		`
		var isOpen bool
		err = tx.GetContext(ctx, &isOpen, selectQuery, account)
		if err != nil {
			return err
		}
		if !isOpen {
			return balance.ErrAccountClosed
		}
		return nil
	})
}

func dbMarkAsClosed(ctx context.Context, db *sqlx.DB, account string) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + openCloseLocksTableName + `
			(token_account, is_open)
			VALUES ($1, FALSE)

			ON CONFLICT (token_account)
			DO UPDATE
				SET is_open = FALSE
				WHERE ` + openCloseLocksTableName + `.token_account = $1 

			RETURNING is_open
		`
		var isOpen bool
		err := tx.GetContext(ctx, &isOpen, query, account)
		if err != nil {
			return err
		}
		if isOpen {
			return errors.New("unexpected state transition")
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
