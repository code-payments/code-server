package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/vault"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	vaultTableName = "codewallet__core_keyvault"
)

type vaultModel struct {
	Id         sql.NullInt64 `db:"id"`
	PublicKey  string        `db:"public_key"`
	PrivateKey string        `db:"private_key"`
	State      uint          `db:"state"`
	CreatedAt  time.Time     `db:"created_at"`
}

func toKeyModel(obj *vault.Record) (*vaultModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	return &vaultModel{
		Id:         sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		PublicKey:  obj.PublicKey,
		PrivateKey: obj.PrivateKey,
		State:      uint(obj.State),
		CreatedAt:  obj.CreatedAt,
	}, nil
}

func fromKeyModel(obj *vaultModel) *vault.Record {

	return &vault.Record{
		Id:         uint64(obj.Id.Int64),
		PublicKey:  obj.PublicKey,
		PrivateKey: obj.PrivateKey,
		State:      vault.State(obj.State),
		CreatedAt:  obj.CreatedAt.UTC(),
	}
}

func (m *vaultModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + vaultTableName + `
		(public_key, private_key, state, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (public_key)
		DO UPDATE
			SET state = $3
			WHERE ` + vaultTableName + `.public_key = $1 
		RETURNING
			id, public_key, private_key, state, created_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.PublicKey,
		m.PrivateKey,
		m.State,
		m.CreatedAt,
	).StructScan(m)

	return pgutil.CheckNoRows(err, vault.ErrInvalidKey)
}

func dbGetCount(ctx context.Context, db *sqlx.DB) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + vaultTableName
	err := db.GetContext(ctx, &res, query)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByState(ctx context.Context, db *sqlx.DB, state vault.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + vaultTableName + ` WHERE state = $1`
	err := db.GetContext(ctx, &res, query, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetKey(ctx context.Context, db *sqlx.DB, pubkey string) (*vaultModel, error) {
	res := &vaultModel{}

	query := `SELECT
		id, public_key, private_key, state, created_at
		FROM ` + vaultTableName + `
		WHERE public_key = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, pubkey)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, vault.ErrKeyNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state vault.State, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*vaultModel, error) {
	res := []*vaultModel{}

	query := `SELECT
		id, public_key, private_key, state, created_at
		FROM ` + vaultTableName + `
		WHERE (state = $1)
	`

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, vault.ErrKeyNotFound)
	}

	if len(res) == 0 {
		return nil, vault.ErrKeyNotFound
	}

	return res, nil
}
