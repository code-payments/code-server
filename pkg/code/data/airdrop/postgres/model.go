package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_airdropeligibility"
)

var (
	errNotFound = errors.New("airdrop eligibility record not found")
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Owner      string `db:"owner"`
	IsEligible bool   `db:"is_eligible"`

	CreatedAt time.Time `db:"created_at"`
}

func toIneligibleModel(owner string) (*model, error) {
	if len(owner) == 0 {
		return nil, errors.New("owner is required")
	}

	return &model{
		Owner:      owner,
		IsEligible: false,
		CreatedAt:  time.Now(),
	}, nil
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(owner, is_eligible, created_at)
			VALUES ($1, $2, $3)

			ON CONFLICT (owner)
			DO UPDATE
				SET is_eligible = $2
				WHERE ` + tableName + `.owner = $1 

			RETURNING id, owner, is_eligible, created_at`

		return tx.QueryRowxContext(
			ctx,
			query,
			m.Owner,
			m.IsEligible,
			m.CreatedAt.UTC(),
		).StructScan(m)
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, owner string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, owner, is_eligible, created_at
		FROM ` + tableName + `
		WHERE owner = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, owner)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, errNotFound)
	}
	return res, nil
}
