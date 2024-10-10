package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/treasury"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	treasuryPoolTableName = "codewallet__core_treasurypool"
	recentRootTableName   = "codewallet__core_treasurypoolrecentroot"
	fundingTableName      = "codewallet__core_treasurypoolfunding"
)

type treasuryPoolModel struct {
	Id sql.NullInt64 `db:"id"`

	Vm string `db:"vm"`

	Name string `db:"name"`

	Address string `db:"address"`
	Bump    uint   `db:"bump"`

	Vault     string `db:"vault"`
	VaultBump uint   `db:"vault_bump"`

	Authority string `db:"authority"`

	MerkleTreeLevels uint `db:"merkle_tree_levels"`

	CurrentIndex    uint `db:"current_index"`
	HistoryListSize uint `db:"history_list_size"`
	HistoryList     []*recentRoot

	SolanaBlock uint64 `db:"solana_block"`

	State uint `db:"state"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

type recentRoot struct {
	Id            sql.NullInt64 `db:"id"`
	Pool          string        `db:"pool"`
	Index         uint          `db:"index"`
	RecentRoot    string        `db:"recent_root"`
	AtSolanaBlock uint64        `db:"at_solana_block"` // explicitly keeping a history of recent root state
}

type fundingModel struct {
	Id            sql.NullInt64 `db:"id"`
	Vault         string        `db:"vault"`
	DeltaQuarks   int64         `db:"delta_quarks"`
	TransactionId string        `db:"transaction_id"`
	State         uint          `db:"state"`
	CreatedAt     time.Time     `db:"created_at"`
}

func toTreasuryPoolModel(obj *treasury.Record) (*treasuryPoolModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	historyList := make([]*recentRoot, obj.HistoryListSize)
	for i, historyItem := range obj.HistoryList {
		historyList[i] = &recentRoot{
			Pool:          obj.Address,
			Index:         uint(i),
			RecentRoot:    historyItem,
			AtSolanaBlock: obj.SolanaBlock,
		}
	}

	return &treasuryPoolModel{
		Vm: obj.Vm,

		Name: obj.Name,

		Address: obj.Address,
		Bump:    uint(obj.Bump),

		Vault:     obj.Vault,
		VaultBump: uint(obj.VaultBump),

		Authority: obj.Authority,

		MerkleTreeLevels: uint(obj.MerkleTreeLevels),

		CurrentIndex:    uint(obj.CurrentIndex),
		HistoryListSize: uint(obj.HistoryListSize),
		HistoryList:     historyList,

		SolanaBlock: obj.SolanaBlock,

		State: uint(obj.State),

		LastUpdatedAt: obj.LastUpdatedAt,
	}, nil
}

func fromTreasuryPoolModel(obj *treasuryPoolModel) *treasury.Record {
	historyList := make([]string, obj.HistoryListSize)
	for _, historyItem := range obj.HistoryList {
		historyList[historyItem.Index] = historyItem.RecentRoot
	}

	return &treasury.Record{
		Id: uint64(obj.Id.Int64),

		Vm: obj.Vm,

		Name: obj.Name,

		Address: obj.Address,
		Bump:    uint8(obj.Bump),

		Vault:     obj.Vault,
		VaultBump: uint8(obj.VaultBump),

		Authority: obj.Authority,

		MerkleTreeLevels: uint8(obj.MerkleTreeLevels),

		CurrentIndex:    uint8(obj.CurrentIndex),
		HistoryListSize: uint8(obj.HistoryListSize),
		HistoryList:     historyList,

		SolanaBlock: obj.SolanaBlock,

		State: treasury.TreasuryPoolState(obj.State),

		LastUpdatedAt: obj.LastUpdatedAt,
	}
}

func (m *treasuryPoolModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		m.LastUpdatedAt = time.Now()

		query := `INSERT INTO ` + treasuryPoolTableName + `
			(vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (address)
			DO UPDATE
				SET current_index = $9, solana_block = $11, last_updated_at = $13
				WHERE ` + treasuryPoolTableName + `.address = $3 AND ` + treasuryPoolTableName + `.vault = $5 AND ` + treasuryPoolTableName + `.solana_block < $11
			RETURNING id, vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at
		`
		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Vm,
			m.Name,
			m.Address,
			m.Bump,
			m.Vault,
			m.VaultBump,
			m.Authority,
			m.MerkleTreeLevels,
			m.CurrentIndex,
			m.HistoryListSize,
			m.SolanaBlock,
			m.State,
			m.LastUpdatedAt.UTC(),
		).StructScan(m)
		if err != nil {
			return pgutil.CheckNoRows(err, treasury.ErrStaleTreasuryPoolState)
		}

		query = `INSERT INTO ` + recentRootTableName + `
			(pool, index, recent_root, at_solana_block)
			VALUES ($1,$2,$3,$4)
			RETURNING id, pool, index, recent_root, at_solana_block
		`
		for _, historyItem := range m.HistoryList {
			err := tx.QueryRowxContext(
				ctx,
				query,
				historyItem.Pool,
				historyItem.Index,
				historyItem.RecentRoot,
				historyItem.AtSolanaBlock,
			).StructScan(historyItem)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (m *treasuryPoolModel) populateHistoryList(ctx context.Context, db *sqlx.DB) error {
	query := `SELECT id, pool, index, recent_root, at_solana_block FROM ` + recentRootTableName + `
		WHERE pool = $1 AND at_solana_block = $2
		ORDER BY index ASC
	`
	err := db.SelectContext(ctx, &m.HistoryList, query, m.Address, m.SolanaBlock)
	if err != nil {
		return err
	}

	if len(m.HistoryList) != int(m.HistoryListSize) {
		return errors.New("unexpected db inconsistency")
	}

	return nil
}

func toFundingModel(obj *treasury.FundingHistoryRecord) (*fundingModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &fundingModel{
		Vault:         obj.Vault,
		DeltaQuarks:   obj.DeltaQuarks,
		TransactionId: obj.TransactionId,
		State:         uint(obj.State),
		CreatedAt:     obj.CreatedAt,
	}, nil
}

func fromFundingModel(m *fundingModel) *treasury.FundingHistoryRecord {
	return &treasury.FundingHistoryRecord{
		Id:            uint64(m.Id.Int64),
		Vault:         m.Vault,
		DeltaQuarks:   m.DeltaQuarks,
		TransactionId: m.TransactionId,
		State:         treasury.FundingState(m.State),
		CreatedAt:     m.CreatedAt,
	}
}

func (m *fundingModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + fundingTableName + `
			(vault, delta_quarks, transaction_id, state, created_at)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (transaction_id)
				DO UPDATE
				SET state = $4
				WHERE ` + fundingTableName + `.transaction_id = $3
			RETURNING id, vault, delta_quarks, transaction_id, state, created_at
		`

	return db.QueryRowxContext(
		ctx,
		query,
		m.Vault,
		m.DeltaQuarks,
		m.TransactionId,
		m.State,
		m.CreatedAt,
	).StructScan(m)
}

func dbGetByName(ctx context.Context, db *sqlx.DB, name string) (*treasuryPoolModel, error) {
	var res treasuryPoolModel
	query := `SELECT id, vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at FROM ` + treasuryPoolTableName + `
		WHERE name = $1
	`
	err := db.GetContext(ctx, &res, query, name)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, treasury.ErrTreasuryPoolNotFound)
	}

	err = res.populateHistoryList(ctx, db)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func dbGetByAddress(ctx context.Context, db *sqlx.DB, address string) (*treasuryPoolModel, error) {
	var res treasuryPoolModel
	query := `SELECT id, vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at FROM ` + treasuryPoolTableName + `
		WHERE address = $1
	`
	err := db.GetContext(ctx, &res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, treasury.ErrTreasuryPoolNotFound)
	}

	err = res.populateHistoryList(ctx, db)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func dbGetByVault(ctx context.Context, db *sqlx.DB, vault string) (*treasuryPoolModel, error) {
	var res treasuryPoolModel
	query := `SELECT id, vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at FROM ` + treasuryPoolTableName + `
		WHERE vault = $1
	`
	err := db.GetContext(ctx, &res, query, vault)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, treasury.ErrTreasuryPoolNotFound)
	}

	err = res.populateHistoryList(ctx, db)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state treasury.TreasuryPoolState, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*treasuryPoolModel, error) {
	res := []*treasuryPoolModel{}

	query := `SELECT id, vm, name, address, bump, vault, vault_bump, authority, merkle_tree_levels, current_index, history_list_size, solana_block, state, last_updated_at
		FROM ` + treasuryPoolTableName + `
		WHERE (state = $1)
	`

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, treasury.ErrTreasuryPoolNotFound)
	}

	if len(res) == 0 {
		return nil, treasury.ErrTreasuryPoolNotFound
	}

	for _, m := range res {
		err = m.populateHistoryList(ctx, db)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func dbGetTotalAvailableFunds(ctx context.Context, db *sqlx.DB, vault string) (uint64, error) {
	var res int64

	query := `SELECT
		(SELECT COALESCE(SUM(delta_quarks), 0) FROM ` + fundingTableName + ` WHERE vault = $1 AND delta_quarks > 0 AND state = $2) +
		(SELECT COALESCE(SUM(delta_quarks), 0) FROM ` + fundingTableName + ` WHERE vault = $1 AND delta_quarks < 0 AND state != $3);
	`

	err := db.GetContext(
		ctx,
		&res,
		query,
		vault,
		treasury.FundingStateConfirmed,
		treasury.FundingStateFailed,
	)
	if err != nil {
		return 0, pgutil.CheckNoRows(err, treasury.ErrTreasuryPoolNotFound)
	}

	if res < 0 {
		return 0, treasury.ErrNegativeFunding
	}
	return uint64(res), nil
}
