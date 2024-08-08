package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	tableName = "codewallet__core_commitment"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Address string `db:"address"`

	Pool       string `db:"pool"`
	RecentRoot string `db:"recent_root"`

	Transcript  string `db:"transcript"`
	Destination string `db:"destination"`
	Amount      uint64 `db:"amount"`

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

	return &model{
		Address: obj.Address,

		Pool:       obj.Pool,
		RecentRoot: obj.RecentRoot,

		Transcript:  obj.Transcript,
		Destination: obj.Destination,
		Amount:      obj.Amount,

		Intent:   obj.Intent,
		ActionId: uint(obj.ActionId),

		Owner: obj.Owner,

		TreasuryRepaid: obj.TreasuryRepaid,
		RepaymentDivertedTo: sql.NullString{
			Valid:  obj.RepaymentDivertedTo != nil,
			String: *pointer.StringOrDefault(obj.RepaymentDivertedTo, ""),
		},

		State: uint(obj.State),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *commitment.Record {
	return &commitment.Record{
		Id: uint64(obj.Id.Int64),

		Address: obj.Address,

		Pool:       obj.Pool,
		RecentRoot: obj.RecentRoot,

		Transcript:  obj.Transcript,
		Destination: obj.Destination,
		Amount:      obj.Amount,

		Intent:   obj.Intent,
		ActionId: uint32(obj.ActionId),

		Owner: obj.Owner,

		TreasuryRepaid:      obj.TreasuryRepaid,
		RepaymentDivertedTo: pointer.StringIfValid(obj.RepaymentDivertedTo.Valid, obj.RepaymentDivertedTo.String),

		State: commitment.State(obj.State),

		CreatedAt: obj.CreatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		divertedToCondition := tableName + ".repayment_diverted_to IS NULL"
		if m.RepaymentDivertedTo.Valid {
			divertedToCondition = "(" + tableName + ".repayment_diverted_to IS NULL OR " + tableName + ".repayment_diverted_to = $11)"
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
		(address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)

		ON CONFLICT (address)
		DO UPDATE
			SET treasury_repaid = $10 OR ` + tableName + `.treasury_repaid , repayment_diverted_to = COALESCE($11, ` + tableName + `.repayment_diverted_to), state = GREATEST($12, ` + tableName + `.state)
			WHERE ` + tableName + `.address = $1 AND ` + treasuryRepaidCondition + ` AND ` + divertedToCondition + ` AND ` + tableName + `.state <= $12

		RETURNING
			id, address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Address,
			m.Pool,
			m.RecentRoot,
			m.Transcript,
			m.Destination,
			m.Amount,
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

	query := `SELECT id, address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
		FROM ` + tableName + `
		WHERE address = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, commitment.ErrCommitmentNotFound)
	}
	return res, nil
}

func dbGetByAction(ctx context.Context, db *sqlx.DB, intentId string, actionId uint32) (*model, error) {
	res := &model{}

	query := `SELECT id, address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
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

	query := `SELECT id, address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
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

	query := `SELECT id, address, pool, recent_root, transcript, destination, amount, intent, action_id, owner, treasury_repaid, repayment_diverted_to, state, created_at
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

func dbCountPendingRepaymentsDivertedToCommitment(ctx context.Context, db *sqlx.DB, commitment string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE repayment_diverted_to = $1 AND NOT treasury_repaid
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		commitment,
	)
	if err != nil {
		return 0, err
	}

	return res, nil
}
