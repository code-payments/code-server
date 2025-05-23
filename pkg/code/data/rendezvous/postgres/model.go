package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_rendezvous"
)

type model struct {
	Id        sql.NullInt64 `db:"id"`
	Key       string        `db:"key"`
	Address   string        `db:"address"`
	CreatedAt time.Time     `db:"created_at"`
	ExpiresAt time.Time     `db:"expires_at"`
}

func toModel(obj *rendezvous.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Key:       obj.Key,
		Address:   obj.Address,
		CreatedAt: obj.CreatedAt,
		ExpiresAt: obj.ExpiresAt,
	}, nil
}

func fromModel(obj *model) *rendezvous.Record {
	return &rendezvous.Record{
		Id:        uint64(obj.Id.Int64),
		Key:       obj.Key,
		Address:   obj.Address,
		CreatedAt: obj.CreatedAt,
		ExpiresAt: obj.ExpiresAt,
	}
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(key, address, created_at, expires_at)
		VALUES ($1, $2, $3, $4)

		ON CONFLICT (key)
		DO UPDATE
			SET address = $2, expires_at = $4
			WHERE ` + tableName + `.key = $1 AND ` + tableName + `.expires_at < NOW()

		RETURNING id, key, address, created_at, expires_at
	`

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	err := db.QueryRowxContext(
		ctx,
		query,
		m.Key,
		m.Address,
		m.CreatedAt,
		m.ExpiresAt,
	).StructScan(m)

	if err != nil {
		return pgutil.CheckNoRows(err, rendezvous.ErrExists)
	}

	return nil
}

func dbExtendExpiry(ctx context.Context, db *sqlx.DB, key, address string, expiry time.Time) error {
	query := `UPDATE ` + tableName + `
		SET expires_at = $1
		WHERE key = $2 AND address = $3 AND expires_at > NOW()
	`

	res, err := db.ExecContext(ctx, query, expiry, key, address)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	} else if rowsAffected == 0 {
		return rendezvous.ErrNotFound
	}
	return nil
}

func dbDelete(ctx context.Context, db *sqlx.DB, key, address string) error {
	query := `DELETE FROM ` + tableName + `
		WHERE key = $1 AND address = $2
	`

	_, err := db.ExecContext(ctx, query, key, address)
	return err
}

func dbGetByKey(ctx context.Context, db *sqlx.DB, key string) (*model, error) {
	var res model
	query := `SELECT id, key, address, created_at, expires_at FROM ` + tableName + `
		WHERE key = $1 AND expires_at > NOW()
	`

	err := db.GetContext(ctx, &res, query, key)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, rendezvous.ErrNotFound)
	}
	return &res, nil
}
