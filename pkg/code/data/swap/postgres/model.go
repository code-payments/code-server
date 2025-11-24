package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/swap"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	tableName = "codewallet__core_swap"
)

type model struct {
	Id                   sql.NullInt64  `db:"id"`
	SwapId               string         `db:"swap_id"`
	Owner                string         `db:"owner"`
	FromMint             string         `db:"from_mint"`
	ToMint               string         `db:"to_mint"`
	Amount               uint64         `db:"amount"`
	FundingId            string         `db:"funding_id"`
	FundingSource        uint8          `db:"funding_source"`
	Nonce                string         `db:"nonce"`
	Blockhash            string         `db:"blockhash"`
	ProofSignature       string         `db:"proof_signature"`
	TransactionSignature sql.NullString `db:"transaction_signature"`
	TransactionBlob      []byte         `db:"transaction_blob"`
	State                uint8          `db:"state"`
	Version              uint64         `db:"version"`
	CreatedAt            time.Time      `db:"created_at"`
}

func toModel(obj *swap.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	return &model{
		Id:                   sql.NullInt64{Int64: int64(obj.Id), Valid: true},
		SwapId:               obj.SwapId,
		Owner:                obj.Owner,
		FromMint:             obj.FromMint,
		ToMint:               obj.ToMint,
		Amount:               obj.Amount,
		FundingId:            obj.FundingId,
		FundingSource:        uint8(obj.FundingSource),
		Nonce:                obj.Nonce,
		Blockhash:            obj.Blockhash,
		ProofSignature:       obj.ProofSignature,
		TransactionSignature: sql.NullString{String: *pointer.StringOrDefault(obj.TransactionSignature, ""), Valid: obj.TransactionSignature != nil},
		TransactionBlob:      obj.TransactionBlob,
		State:                uint8(obj.State),
		Version:              obj.Version,
		CreatedAt:            obj.CreatedAt,
	}, nil
}

func fromModel(m *model) *swap.Record {
	return &swap.Record{
		Id:                   uint64(m.Id.Int64),
		SwapId:               m.SwapId,
		Owner:                m.Owner,
		FromMint:             m.FromMint,
		ToMint:               m.ToMint,
		Amount:               m.Amount,
		FundingId:            m.FundingId,
		FundingSource:        swap.FundingSource(m.FundingSource),
		Nonce:                m.Nonce,
		Blockhash:            m.Blockhash,
		ProofSignature:       m.ProofSignature,
		TransactionSignature: pointer.StringIfValid(m.TransactionSignature.Valid, m.TransactionSignature.String),
		TransactionBlob:      m.TransactionBlob,
		State:                swap.State(m.State),
		Version:              m.Version,
		CreatedAt:            m.CreatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14 + 1, $15)

			ON CONFLICT (swap_id)
			DO UPDATE
				SET transaction_signature = $11, transaction_blob = $12, state = $13, version = ` + tableName + `.version + 1
				WHERE ` + tableName + `.swap_id = $1 AND ` + tableName + `.version = $14

			RETURNING
				id, swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at`

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.SwapId,
			m.Owner,
			m.FromMint,
			m.ToMint,
			m.Amount,
			m.FundingId,
			m.FundingSource,
			m.Nonce,
			m.Blockhash,
			m.ProofSignature,
			m.TransactionSignature,
			m.TransactionBlob,
			m.State,
			m.Version,
			m.CreatedAt,
		).StructScan(m)
		if err != nil {
			return pgutil.CheckNoRows(err, swap.ErrStaleVersion)
		}
		return nil
	})
}

func dbGetById(ctx context.Context, db *sqlx.DB, id string) (*model, error) {
	res := &model{}

	query := `SELECT id, swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at
		FROM ` + tableName + `
		WHERE swap_id = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, id)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, swap.ErrNotFound)
	}
	return res, nil
}

func dbGetByFundingId(ctx context.Context, db *sqlx.DB, fundingId string) (*model, error) {
	res := &model{}

	query := `SELECT id, swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at
		FROM ` + tableName + `
		WHERE funding_id = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, fundingId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, swap.ErrNotFound)
	}
	return res, nil
}

func dbGetAllByOwnerAndState(ctx context.Context, db *sqlx.DB, owner string, state swap.State) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at
		FROM ` + tableName + `
		WHERE owner = $1 AND state = $2`

	err := db.SelectContext(ctx, &res, query, owner, state)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, swap.ErrNotFound)
	}

	if len(res) == 0 {
		return nil, swap.ErrNotFound
	}

	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state swap.State, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*model, error) {
	res := []*model{}

	query := `SELECT
		id, swap_id, owner, from_mint, to_mint, amount, funding_id, funding_source, nonce, blockhash, proof_signature, transaction_signature, transaction_blob, state, version, created_at
		FROM ` + tableName + `
		WHERE state = $1`

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, swap.ErrNotFound)
	}

	if len(res) == 0 {
		return nil, swap.ErrNotFound
	}
	return res, nil
}
