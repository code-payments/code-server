package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/badgecount"
)

const (
	tableName = "codewallet__core_badgecount"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Owner      string `db:"owner"`
	BadgeCount uint32 `db:"badge_count"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
	CreatedAt     time.Time `db:"created_at"`
}

func fromModel(obj *model) *badgecount.Record {
	return &badgecount.Record{
		Id: uint64(obj.Id.Int64),

		Owner:      obj.Owner,
		BadgeCount: obj.BadgeCount,

		LastUpdatedAt: obj.LastUpdatedAt,
		CreatedAt:     obj.CreatedAt,
	}
}

func dbAdd(ctx context.Context, db *sqlx.DB, owner string, amount uint32) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		m := &model{
			Owner:      owner,
			BadgeCount: amount,

			LastUpdatedAt: time.Now(),
			CreatedAt:     time.Now(),
		}

		query := `INSERT INTO ` + tableName + `
			(owner, badge_count, last_updated_at, created_at)
			VALUES ($1, $2, $3, $4)

			ON CONFLICT (owner)
			DO UPDATE
				SET badge_count = ` + tableName + `.badge_count + $2, last_updated_at = $3
				WHERE ` + tableName + `.owner = $1 

			RETURNING
				id, owner, badge_count, last_updated_at, created_at`

		_, err := tx.ExecContext(
			ctx,
			query,
			m.Owner,
			m.BadgeCount,
			m.LastUpdatedAt,
			m.CreatedAt,
		)
		return err
	})
}

func dbReset(ctx context.Context, db *sqlx.DB, owner string) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `UPDATE ` + tableName + `
			SET badge_count = 0, last_updated_at = $2
			WHERE owner = $1
		`

		_, err := tx.ExecContext(
			ctx,
			query,
			owner,
			time.Now(),
		)
		return err
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, owner string) (*model, error) {
	res := &model{}

	query := `SELECT id, owner, badge_count, last_updated_at, created_at FROM ` + tableName + `
		WHERE owner = $1
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		owner,
	).StructScan(res)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, badgecount.ErrBadgeCountNotFound)
	}
	return res, nil
}
