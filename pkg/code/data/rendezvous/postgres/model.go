package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
)

const (
	tableName = "codewallet__core_rendezvous"
)

type model struct {
	Id            sql.NullInt64 `db:"id"`
	Key           string        `db:"key"`
	Location      string        `db:"location"`
	CreatedAt     time.Time     `db:"created_at"`
	LastUpdatedAt time.Time     `db:"last_updated_at"`
}

func toModel(obj *rendezvous.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Key:           obj.Key,
		Location:      obj.Location,
		CreatedAt:     obj.CreatedAt,
		LastUpdatedAt: obj.LastUpdatedAt,
	}, nil
}

func fromModel(obj *model) *rendezvous.Record {
	return &rendezvous.Record{
		Id:            uint64(obj.Id.Int64),
		Key:           obj.Key,
		Location:      obj.Location,
		CreatedAt:     obj.CreatedAt,
		LastUpdatedAt: obj.LastUpdatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(key, location, created_at, last_updated_at)
		VALUES ($1, $2, $3, $4)

		ON CONFLICT (key)
		DO UPDATE
			SET location = $2, last_updated_at = $4
			WHERE ` + tableName + `.key = $1 

		RETURNING id, key, location, created_at, last_updated_at
	`

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	m.LastUpdatedAt = time.Now()

	return db.QueryRowxContext(
		ctx,
		query,
		m.Key,
		m.Location,
		m.CreatedAt,
		m.LastUpdatedAt,
	).StructScan(m)
}

func dbGetByKey(ctx context.Context, db *sqlx.DB, key string) (*model, error) {
	var res model
	query := `SELECT id, key, location, created_at, last_updated_at FROM ` + tableName + `
		WHERE key = $1
	`

	err := db.GetContext(ctx, &res, query, key)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, rendezvous.ErrNotFound)
	}
	return &res, nil
}

func dbDelete(ctx context.Context, db *sqlx.DB, key string) error {
	query := `DELETE FROM ` + tableName + `
		WHERE key = $1
	`

	_, err := db.ExecContext(ctx, query, key)
	return err
}
