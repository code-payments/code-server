package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

const (
	tableName = "codewallet__core_commitment"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	DataVersion uint `db:"data_version"`

	Address string `db:"address"`
	Bump    uint   `db:"bump"`

	Pool       string `db:"pool"`
	PoolBump   uint   `db:"pool_bump"`
	RecentRoot string `db:"recent_root"`
	Transcript string `db:"transcript"`

	Destination string `db:"destination"`
	Amount      uint64 `db:"amount"`

	Vault     string `db:"vault"`
	VaultBump uint   `db:"vault_bump"`

	Intent   string `db:"intent"`
	ActionId uint   `db:"action_id"`

	Owner string `db:"owner"`

	TreasuryRepaid      bool           `db:"treasury_repaid"`
	RepaymentDivertedTo sql.NullString `db:"repayment_diverted_to"`

	State uint `db:"state"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *commitment.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	var repaymentDivertedTo sql.NullString
	if obj.RepaymentDivertedTo != nil {
		repaymentDivertedTo.Valid = true
		repaymentDivertedTo.String = *obj.RepaymentDivertedTo
	}

	return &model{
		DataVersion: uint(obj.DataVersion),

		Address: obj.Address,
		Bump:    uint(obj.Bump),

		Pool:       obj.Pool,
		PoolBump:   uint(obj.PoolBump),
		RecentRoot: obj.RecentRoot,
		Transcript: obj.Transcript,

		Destination: obj.Destination,
		Amount:      obj.Amount,

		Vault:     obj.Vault,
		VaultBump: uint(obj.VaultBump),

		Intent:   obj.Intent,
		ActionId: uint(obj.ActionId),

		Owner: obj.Owner,

		TreasuryRepaid:      obj.TreasuryRepaid,
		RepaymentDivertedTo: repaymentDivertedTo,

		State: uint(obj.State),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *commitment.Record {
	record := &commitment.Record{
		Id: uint64(obj.Id.Int64),

		DataVersion: splitter_token.DataVersion(obj.DataVersion),

		Address: obj.Address,
		Bump:    uint8(obj.Bump),

		Pool:       obj.Pool,
		PoolBump:   uint8(obj.PoolBump),
		RecentRoot: obj.RecentRoot,
		Transcript: obj.Transcript,

		Destination: obj.Destination,
		Amount:      obj.Amount,

		Vault:     obj.Vault,
		VaultBump: uint8(obj.VaultBump),

		Intent:   obj.Intent,
		ActionId: uint32(obj.ActionId),

		Owner: obj.Owner,

		TreasuryRepaid: obj.TreasuryRepaid,

		State: commitment.State(obj.State),

		CreatedAt: obj.CreatedAt,
	}

	if obj.RepaymentDivertedTo.Valid {
		record.RepaymentDivertedTo = &obj.RepaymentDivertedTo.String
	}

	return record
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		divertedToCondition := tableName + ".repayment_diverted_to IS NULL"
		if m.RepaymentDivertedTo.Valid {
			divertedToCondition = "(" + tableName + ".repayment_diverted_to IS NULL OR " + tableName + ".repayment_diverted_to = $16)"
		}

		treasuryRepaidCondition := tableName + ".treasury_repaid IS FALSE"
		if m.TreasuryRepaid {
			treasuryRepaidCondition = `TRUE`
		}

		// If you're wondering why this is so safeguarded, commitments are at a
		// high risk of experiencing race conditions without distributed locks.
		// Luckily, all updateable state-like fields should progress forward in a
		// predictable manner, making conditions easy to reason about.
		query := `INSERT INTO ` + tableName + `
		(data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)

		ON CONFLICT (address)
		DO UPDATE
			SET treasury_repaid = $15 OR ` + tableName + `.treasury_repaid , repayment_diverted_to = COALESCE($16, ` + tableName + `.repayment_diverted_to), state = GREATEST($17, ` + tableName + `.state)
			WHERE ` + tableName + `.address = $2 AND ` + treasuryRepaidCondition + ` AND ` + divertedToCondition + ` AND ` + tableName + `.state <= $17

		RETURNING
			id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.DataVersion,
			m.Address,
			m.Bump,
			m.Pool,
			m.PoolBump,
			m.RecentRoot,
			m.Transcript,
			m.Destination,
			m.Amount,
			m.Vault,
			m.VaultBump,
			m.Intent,
			m.ActionId,
			m.Owner,
			m.TreasuryRepaid,
			m.RepaymentDivertedTo,
			m.State,
			m.CreatedAt.UTC(),
		).StructScan(m)

		return pgutil.CheckNoRows(err, commitment.ErrInvalidCommitment)
	})
}

func dbGetByAddress(ctx context.Context, db *sqlx.DB, address string) (*model, error) {
	res := &model{}

	query := `SELECT id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE address = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}
	return res, nil
}

func dbGetByVault(ctx context.Context, db *sqlx.DB, vault string) (*model, error) {
	res := &model{}

	query := `SELECT id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE vault = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, vault)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}
	return res, nil
}

func dbGetByAction(ctx context.Context, db *sqlx.DB, intentId string, actionId uint32) (*model, error) {
	res := &model{}

	query := `SELECT id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE intent = $1 AND action_id = $2
		LIMIT 1`

	err := db.GetContext(ctx, res, query, intentId, actionId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state commitment.State, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE (state = $1)
	`

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}

	if len(res) == 0 {
		return nil, commitment.ErrCommitmentNotFound
	}
	return res, nil
}

func dbGetUpgradeableByOwner(ctx context.Context, db *sqlx.DB, owner string, limit uint64) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, data_version, address, bump, pool, pool_bump, recent_root, transcript, destination, amount, vault, vault_bump, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE owner = $1 AND state > $3 AND state < $4 AND repayment_diverted_to IS NULL
		LIMIT $2
	`

	err := db.SelectContext(ctx, &res, query, owner, limit, commitment.StatePayingDestination, commitment.StateReadyToRemoveFromMerkleTree)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}

	if len(res) == 0 {
		return nil, commitment.ErrCommitmentNotFound
	}
	return res, nil
}

func dbGetUsedTreasuryPoolDeficit(ctx context.Context, db *sqlx.DB, pool string) (uint64, error) {
	var res uint64

	query := `SELECT COALESCE(SUM(amount), 0) FROM ` + tableName + `
		WHERE pool = $1 AND NOT treasury_repaid AND state != $2
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		pool,
		commitment.StateUnknown,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetTotalTreasuryPoolDeficit(ctx context.Context, db *sqlx.DB, pool string) (uint64, error) {
	var res uint64

	query := `SELECT COALESCE(SUM(amount), 0) FROM ` + tableName + `
		WHERE pool = $1 AND NOT treasury_repaid
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		pool,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbCountByState(ctx context.Context, db *sqlx.DB, state commitment.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE state = $1
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		state,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbCountRepaymentsDivertedToVault(ctx context.Context, db *sqlx.DB, vault string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE repayment_diverted_to = $1
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		vault,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}
