package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/pointer"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	nonceTableName = "codewallet__core_nonce"
)

type nonceModel struct {
	Id                  sql.NullInt64  `db:"id"`
	Address             string         `db:"address"`
	Authority           string         `db:"authority"`
	Blockhash           string         `db:"blockhash"`
	Environment         uint           `db:"environment"`
	EnvironmentInstance string         `db:"environment_instance"`
	Purpose             uint           `db:"purpose"`
	State               uint           `db:"state"`
	Signature           string         `db:"signature"`
	ClaimNodeId         sql.NullString `db:"claim_node_id"`
	ClaimExpiresAtMs    sql.NullInt64  `db:"claim_expires_at"`
	Version             int64          `db:"version"`
}

func toNonceModel(obj *nonce.Record) (*nonceModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	var claimExpiresAtMs int64
	if obj.ClaimExpiresAt != nil {
		claimExpiresAtMs = int64(obj.ClaimExpiresAt.UnixMilli())
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
		ClaimNodeId: sql.NullString{
			Valid:  obj.ClaimNodeID != nil,
			String: *pointer.StringOrDefault(obj.ClaimNodeID, ""),
		},
		ClaimExpiresAtMs: sql.NullInt64{
			Valid: claimExpiresAtMs > 0,
			Int64: claimExpiresAtMs,
		},
		Version: int64(obj.Version),
	}, nil
}

func fromNonceModel(obj *nonceModel) *nonce.Record {
	var claimExpiresAt *time.Time
	if obj.ClaimExpiresAtMs.Valid {
		ts := time.UnixMilli(obj.ClaimExpiresAtMs.Int64)
		claimExpiresAt = &ts
	}

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
		ClaimNodeID:         pointer.StringIfValid(obj.ClaimNodeId.Valid, obj.ClaimNodeId.String),
		ClaimExpiresAt:      claimExpiresAt,
		Version:             uint64(obj.Version),
	}
}

func (m *nonceModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + nonceTableName + `
			(address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11 + 1)
			ON CONFLICT (address)
			DO UPDATE
				SET blockhash = $3, state = $7, signature = $8, claim_node_id = $9, claim_expires_at = $10, version = ` + nonceTableName + `.version + 1
				WHERE ` + nonceTableName + `.address = $1 AND ` + nonceTableName + `.version = $11
			RETURNING
				id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version`

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
			m.ClaimNodeId,
			m.ClaimExpiresAtMs,
			m.Version,
		).StructScan(m)

		return pgutil.CheckNoRows(err, nonce.ErrStaleVersion)
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
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version
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
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version
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
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version
		FROM ` + nonceTableName + `
		WHERE environment = $1 AND environment_instance = $2 AND ((state = $3) OR (state = $4 AND claim_expires_at < $5)) AND purpose = $6 AND signature IS NOT NULL
		OFFSET FLOOR(RANDOM() * 100)
		LIMIT 1
	`
	fallbackQuery := `SELECT
		id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version
		FROM ` + nonceTableName + `
		WHERE environment = $1 AND environment_instance = $2 AND ((state = $3) OR (state = $4 AND claim_expires_at < $5)) AND purpose = $6 AND signature IS NOT NULL
		LIMIT 1
	`

	nowMs := time.Now().UnixMilli()
	err := db.GetContext(ctx, res, query, env, instance, nonce.StateAvailable, nonce.StateClaimed, nowMs, purpose)
	if err != nil {
		err = pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)

		// No nonces found. Because our query isn't perfect, fall back to a
		// strategy that will guarantee to select something if an available
		// nonce exists.
		if err == nonce.ErrNonceNotFound {
			err := db.GetContext(ctx, res, fallbackQuery, env, instance, nonce.StateAvailable, nonce.StateClaimed, nowMs, purpose)
			if err != nil {
				return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
			}
			return res, nil
		}

		return nil, err
	}
	return res, nil
}

func dbBatchClaimAvailableByPurpose(
	ctx context.Context,
	db *sqlx.DB,
	env nonce.Environment,
	instance string,
	purpose nonce.Purpose,
	limit int,
	nodeID string,
	minExpireAt time.Time,
	maxExpireAt time.Time,
) ([]*nonceModel, error) {
	// Signature null check is required because some legacy records didn't have this
	// set and causes this call to fail. This is a result of the field not being
	// defined at the time of record creation.
	//
	// todo: Fix said nonce records
	res := []*nonceModel{}
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `WITH selected_nonces AS (
			SELECT id, address, authority, blockhash, environment, environment_instance, purpose, state, signature, claim_node_id, claim_expires_at, version
			FROM ` + nonceTableName + `
			WHERE environment = $1 AND environment_instance = $2 AND ((state = $3) OR (state = $4 AND claim_expires_at < $5)) AND purpose = $6 AND signature IS NOT NULL
			LIMIT $7
			FOR UPDATE
        )
        UPDATE ` + nonceTableName + `
        SET state = $8, claim_node_id = $9, claim_expires_at = $10 + FLOOR(RANDOM() * $11), version = ` + nonceTableName + `.version + 1
        FROM selected_nonces
        WHERE ` + nonceTableName + `.id = selected_nonces.id
        RETURNING ` + nonceTableName + `.*`

		return tx.SelectContext(
			ctx,
			&res,
			query,
			env,
			instance,
			nonce.StateAvailable,
			nonce.StateClaimed,
			time.Now().UnixMilli(),
			purpose,
			limit,
			nonce.StateClaimed,
			nodeID,
			minExpireAt.UnixMilli(),
			maxExpireAt.Sub(minExpireAt).Milliseconds(),
		)
	})
	if err != nil {
		return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
	}
	if len(res) == 0 {
		return nil, nonce.ErrNonceNotFound
	}
	return res, nil
}
