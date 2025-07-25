package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed timelock.Store
func New(db *sql.DB) timelock.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements timelock.Store.Save
func (s *store) Save(ctx context.Context, record *timelock.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	if err := model.dbSave(ctx, s.db); err != nil {
		return err
	}

	res := fromModel(model)
	res.CopyTo(record)

	return nil
}

// GetByAddress implements timelock.Store.GetByAddress
func (s *store) GetByAddress(ctx context.Context, address string) (*timelock.Record, error) {
	model, err := dbGetByAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetByVault implements timelock.Store.GetByVault
func (s *store) GetByVault(ctx context.Context, vault string) (*timelock.Record, error) {
	model, err := dbGetByVault(ctx, s.db, vault)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetByVaultBatch implements timelock.Store.GetByVaultBatch
func (s *store) GetByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error) {
	models, err := dbGetByVaultBatch(ctx, s.db, vaults...)
	if err != nil {
		return nil, err
	}

	timelocksByVault := make(map[string]*timelock.Record, len(models))
	for _, model := range models {
		timelocksByVault[model.VaultAddress] = fromModel(model)
	}
	return timelocksByVault, nil
}

// GetByDepositPda implements timelock.Store.GetByDepositPda
func (s *store) GetByDepositPda(ctx context.Context, depositPda string) (*timelock.Record, error) {
	model, err := dbGetByDepositPda(ctx, s.db, depositPda)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetOldestByState implements timelock.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state timelock_token.TimelockState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*timelock.Record, error) {
	res, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	timelocks := make([]*timelock.Record, len(res))
	for i, model := range res {
		timelocks[i] = fromModel(model)
	}
	return timelocks, nil
}

// GetCountByState implements timelock.Store.GetCountByState
func (s *store) GetCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error) {
	return dbGetCountByState(ctx, s.db, state)
}
