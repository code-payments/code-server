package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/nonce"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	nonceTableName = "codewallet__core_nonce"
)

type nonceModel struct {
	Id               sql.NullInt64 `db:"id"`
	Address          string        `db:"address"`
	Authority        string        `db:"authority"`
	Blockhash        string        `db:"blockhash"`
	Purpose          uint          `db:"purpose"`
	State            uint          `db:"state"`
	Signature        string        `db:"signature"`
	ClaimNodeId      string        `db:"claim_node_id"`
	ClaimExpiresAtMs int64         `db:"claim_expires_at"`
}

func toNonceModel(obj *nonce.Record) (*nonceModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &nonceModel{
		Id:               sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		Address:          obj.Address,
		Authority:        obj.Authority,
		Blockhash:        obj.Blockhash,
		Purpose:          uint(obj.Purpose),
		State:            uint(obj.State),
		Signature:        obj.Signature,
		ClaimNodeId:      obj.ClaimNodeId,
		ClaimExpiresAtMs: obj.ClaimExpiresAt.UnixMilli(),
	}, nil
}

func fromNonceModel(obj *nonceModel) *nonce.Record {
	return &nonce.Record{
		Id:             uint64(obj.Id.Int64),
		Address:        obj.Address,
		Authority:      obj.Authority,
		Blockhash:      obj.Blockhash,
		Purpose:        nonce.Purpose(obj.Purpose),
		State:          nonce.State(obj.State),
		Signature:      obj.Signature,
		ClaimNodeId:    obj.ClaimNodeId,
		ClaimExpiresAt: time.UnixMilli(obj.ClaimExpiresAtMs),
	}
}

func (m *nonceModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + nonceTableName + `
			(address, authority, blockhash, purpose, state, signature, claim_node_id, claim_expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (address)
			DO UPDATE
				SET blockhash = $3, state = $5, signature = $6
				WHERE ` + nonceTableName + `.address = $1
			RETURNING
				id, address, authority, blockhash, purpose, state, signature`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Address,
			m.Authority,
			m.Blockhash,
			m.Purpose,
			m.State,
			m.Signature,
			m.ClaimNodeId,
			m.ClaimExpiresAtMs,
		).StructScan(m)

		return pgutil.CheckNoRows(err, nonce.ErrInvalidNonce)
	})
}

func dbGetCount(ctx context.Context, db *sqlx.DB) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName
	err := db.GetContext(ctx, &res, query)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByState(ctx context.Context, db *sqlx.DB, state nonce.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName + ` WHERE state = $1`
	err := db.GetContext(ctx, &res, query, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByStateAndPurpose(ctx context.Context, db *sqlx.DB, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + nonceTableName + ` WHERE state = $1 and purpose = $2`
	err := db.GetContext(ctx, &res, query, state, purpose)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetNonce(ctx context.Context, db *sqlx.DB, address string) (*nonceModel, error) {
	res := &nonceModel{}

	query := `SELECT
		id, address, authority, blockhash, purpose, state, signature, claim_node_id, claim_expires_at
		FROM ` + nonceTableName + `
		WHERE address = $1
	`

	err := db.GetContext(ctx, res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state nonce.State, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*nonceModel, error) {
	res := []*nonceModel{}

	// Signature null check is required because some legacy records didn't have this
	// set and causes this call to fail. This is a result of the field not being
	// defined at the time of record creation.
	//
	// todo: Fix said nonce records
	query := `SELECT
		id, address, authority, blockhash, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE (state = $1 AND signature IS NOT NULL)
	`

	opts := []interface{}{state}
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

// We query a random nonce by first selecting any available candidate from the
// total set, applying an upper limit of 100, and _then_ randomly shuffling the
// results and selecting the first. By bounding the size before ORDER BY random(),
// we avoid having to shuffle large sets of results.
//
// Previously, we would use OFFSET FLOOR(RANDOM() * 100). However, if the pool
// (post filter) size was less than 100, any selection > pool size would result
// in the OFFSET being set to zero. This meant random() disappeared for a subset
// of values. In practice, this would result in a bias, and increased contention.
//
// For example, 50 Available nonce's, 25 Claimed (expired), 25 Reserved. With Offset:
//
//  1. 50% of the time would be a random Available.
//  2. 25% of the time would be a random expired Claimed.
//  3. 25% of the time would be _the first_ Available.
//
// This meant that 25% of the time would not be random. As we pull from the pool,
// this % only increases, further causing contention.
//
// Performance wise, this approach is slightly worse, but the vast majority of the
// time is spent on the scan and filter. Below are two example query plans (from a
// small dataset in an online editor).
//
// QUERY PLAN (OFFSET):
//
//	Limit  (cost=17.80..35.60 rows=1 width=140) (actual time=0.019..0.019 rows=0 loops=1)
//	->  Seq Scan on codewallet__core_nonce  (cost=0.00..17.80 rows=1 width=140) (actual time=0.016..0.017 rows=0 loops=1)
//	      Filter: ((signature IS NOT NULL) AND (purpose = 1) AND ((state = 0) OR ((state = 2) AND (claim_expires_at < 200))))
//	      Rows Removed by Filter: 100
//
// Planning Time: 0.046 ms
// Execution Time: 0.031 ms
//
// QUERY PLAN (ORDER BY):
//
//	Limit  (cost=17.82..17.83 rows=1 width=148) (actual time=0.018..0.019 rows=0 loops=1)
//	->  Sort  (cost=17.82..17.83 rows=1 width=148) (actual time=0.018..0.018 rows=0 loops=1)
//	      Sort Key: (random())
//	      Sort Method: quicksort  Memory: 25kB
//	      ->  Subquery Scan on sub  (cost=0.00..17.81 rows=1 width=148) (actual time=0.015..0.016 rows=0 loops=1)
//	            ->  Limit  (cost=0.00..17.80 rows=1 width=140) (actual time=0.015..0.015 rows=0 loops=1)
//	                  ->  Seq Scan on codewallet__core_nonce  (cost=0.00..17.80 rows=1 width=140) (actual time=0.015..0.015 rows=0 loops=1)
//	                        Filter: ((signature IS NOT NULL) AND (purpose = 1) AND ((state = 0) OR ((state = 2) AND (claim_expires_at < 200))))
//	                        Rows Removed by Filter: 100
//
// Planning Time: 0.068 ms
// Execution Time: 0.037 ms
//
// Overall, the Seq Scan and Filter is the bulk of the work, with the ORDER BY RANDOM()
// adding a small (fixed) amount of overhead. The trade-off is negligible time complexity
// for more reliable semantics.
func dbGetRandomAvailableByPurpose(ctx context.Context, db *sqlx.DB, purpose nonce.Purpose) (*nonceModel, error) {
	res := &nonceModel{}
	nowMs := time.Now().UnixMilli()

	// Signature null check is required because some legacy records didn't have this
	// set and causes this call to fail. This is a result of the field not being
	// defined at the time of record creation.
	//
	// todo: Fix said nonce records
	query := `
		SELECT id, address, authority, blockhash, purpose, state, signature FROM (
			SELECT id, address, authority, blockhash, purpose, state, signature
			FROM ` + nonceTableName + `
			WHERE ((state = $1) OR (state = $2 AND claim_expires_at < $3)) AND purpose = $4 AND signature IS NOT NULL
			LIMIT 100
		) sub
		ORDER BY random()
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, nonce.StateAvailable, nonce.StateClaimed, nowMs, purpose)
	return res, pgutil.CheckNoRows(err, nonce.ErrNonceNotFound)
}

func dbBatchClaimAvailableByPurpose(
	ctx context.Context,
	db *sqlx.DB,
	purpose nonce.Purpose,
	limit int,
	nodeId string,
	minExpireAt time.Time,
	maxExpireAt time.Time,
) ([]*nonceModel, error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}

	query := `
	WITH selected_nonces AS (
		SELECT id, address, authority, blockhash, purpose, state, signature
		FROM ` + nonceTableName + `
		WHERE ((state = $1) OR (state = $2 AND claim_expires_at < $3)) AND purpose = $4 AND signature IS NOT NULL
		LIMIT $5
		FOR UPDATE
        )
        UPDATE ` + nonceTableName + `
        SET state = $6,
            claim_node_id = $7,
            claim_expires_at = $8 + FLOOR(RANDOM() * $9)
        FROM selected_nonces
        WHERE ` + nonceTableName + `.id = selected_nonces.id
        RETURNING ` + nonceTableName + `.*
	`

	nonceModels := []*nonceModel{}
	err = tx.SelectContext(
		ctx,
		&nonceModels,
		query,
		nonce.StateAvailable,
		nonce.StateClaimed,
		time.Now().UnixMilli(),
		purpose,
		limit,
		nonce.StateClaimed,
		nodeId,
		minExpireAt.UnixMilli(),
		maxExpireAt.Sub(minExpireAt).Milliseconds(),
	)
	if err != nil {
		if rollBackErr := tx.Rollback(); rollBackErr != nil {
			return nil, fmt.Errorf("failed to rollback (cause: %w): %w", err, rollBackErr)
		}

		return nil, err
	}

	return nonceModels, tx.Commit()
}
