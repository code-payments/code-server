package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/nonce"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	nonceTableName = "codewallet__core_nonce"
)

type nonceModel struct {
	Id                  sql.NullInt64 `db:"id"`
	Address             string        `db:"address"`
	Authority           string        `db:"authority"`
	Blockhash           string        `db:"blockhash"`
	Environment         uint          `db:"environment"`
	EnvironmentInstance string        `db:"environment_instance"`
	Purpose             uint          `db:"purpose"`
	State               uint          `db:"state"`
	Signature           string        `db:"signature"`
}

func toNonceModel(obj *nonce.Record) (*nonceModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &nonceModel{
		Id:                  sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		Address:             obj.Address,
		Authority:           obj.Authority,
		Blockhash:           obj.Blockhash,
		Environment:         uint(obj.Environment),
		EnvironmentInstance: obj.EnvironmentInstance,
		Purpose:             uint(obj.Purpose),
		State:               uint(obj.State),
		Signature:           obj.Signature,
	}, nil
}

func fromNonceModel(obj *nonceModel) *nonce.Record {
	return &nonce.Record{
		Id:                  uint64(obj.Id.Int64),
		Address:             obj.Address,
		Authority:           obj.Authority,
		Blockhash:           obj.Blockhash,
		Environment:         nonce.Environment(obj.Environment),
		EnvironmentInstance: obj.EnvironmentInstance,
		Purpose:             nonce.Purpose(obj.Purpose),
		State:               nonce.State(obj.State),
		Signature:           obj.Signature,
	}
}

func (m *nonceModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + nonceTableName + `
			(address, authority, blockhash, environment, environment_instance, purpose, state, signature)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (address)
			DO UPDATE
				SET blockhash = $3, state = $7, signature = $8
				WHERE ` + nonceTableName + `.address = $1 
			RETURNING
				id, address, authority, blockhash, environment, environment_instance, purpose, state, signature`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Address,
			m.Authority,
			m.Blockhash,
			m.Environment,
			m.EnvironmentInstance,
			m.Purpose,
			m.State,
			m.Signature,
		).StructScan(m)

		return pgutil.CheckNoRows(err, nonce.ErrInvalidNonce)
	})
}

func dbGetCount(ctx context.Context, db *sqlx.DB, env nonce.Environment, instance string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName + ` WHERE environment = $1 AND environment_instance = $2`
	err := db.GetContext(ctx, &res, query, env, instance)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByState(ctx context.Context, db *sqlx.DB, env nonce.Environment, instance string, state nonce.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName + ` WHERE environment = $1 AND environment_instance = $2 AND state = $3`
	err := db.GetContext(ctx, &res, query, env, instance, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByStateAndPurpose(ctx context.Context, db *sqlx.DB, env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName + ` WHERE environment = $1 AND environment_instance = $2 AND state = $3 and purpose = $4`
	err := db.GetContext(ctx, &res, query, env, instance, state, purpose)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetNonce(ctx context.Context, db *sqlx.DB, address string) (*nonceModel, error) {
	res := &nonceModel{}

	query := `SELECT
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE address = $1
	`

	err := db.GetContext(ctx, res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, env nonce.Environment, instance string, state nonce.State, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*nonceModel, error) {
	res := []*nonceModel{}

	// Signature null check is required because some legacy records didn't have this
	// set and causes this call to fail. This is a result of the field not being
	// defined at the time of record creation.
	//
	// todo: Fix said nonce records
	query := `SELECT
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE environment = $1 AND environment_instance = $2 AND state = $3 AND signature IS NOT NULL
	`

	opts := []interface{}{env, instance, state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
	}

	if len(res) == 0 {
		return nil, nonce.ErrNonceNotFound
	}

	return res, nil
}

// todo: Implementation still isn't perfect, but better than no randomness. It's
// sufficiently efficient, as long as our nonce pool is larger than the max offset.
// todo: We may need to tune the offset based on pool size and environment, but it
// should be sufficiently good enough for now.
func dbGetRandomAvailableByPurpose(ctx context.Context, db *sqlx.DB, env nonce.Environment, instance string, purpose nonce.Purpose) (*nonceModel, error) {
	res := &nonceModel{}

	// Signature null check is required because some legacy records didn't have this
	// set and causes this call to fail. This is a result of the field not being
	// defined at the time of record creation.
	//
	// todo: Fix said nonce records
	query := `SELECT
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE environment = $1 AND environment_instance = $2 AND state = $3 AND purpose = $4 AND signature IS NOT NULL
		OFFSET FLOOR(RANDOM() * 100)
		LIMIT 1
	`
	fallbackQuery := `SELECT
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE environment = $1 AND environment_instance = $2 AND state = $3 AND purpose = $4 AND signature IS NOT NULL
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, env, instance, nonce.StateAvailable, purpose)
	if err != nil {
		err = pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)

		// No nonces found. Because our query isn't perfect, fall back to a
		// strategy that will guarantee to select something if an available
		// nonce exists.
		if err == nonce.ErrNonceNotFound {
			err := db.GetContext(ctx, res, fallbackQuery, env, instance, nonce.StateAvailable, purpose)
			if err != nil {
				return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
			}
			return res, nil
		}

		return nil, err
	}
	return res, nil
}
