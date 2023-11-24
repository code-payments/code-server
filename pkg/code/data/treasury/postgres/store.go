package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed treasury.Store
func New(db *sql.DB) treasury.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements treasury.Store.Save
func (s *store) Save(ctx context.Context, record *treasury.Record) error {
	model, err := toTreasuryPoolModel(record)
	if err != nil {
		return err
	}

	if err := model.dbSave(ctx, s.db); err != nil {
		return err
	}

	res := fromTreasuryPoolModel(model)
	res.CopyTo(record)

	return nil
}

// GetByName implements treasury.Store.GetByName
func (s *store) GetByName(ctx context.Context, name string) (*treasury.Record, error) {
	model, err := dbGetByName(ctx, s.db, name)
	if err != nil {
		return nil, err
	}

	return fromTreasuryPoolModel(model), nil
}

// GetByAddress implements treasury.Store.GetByAddress
func (s *store) GetByAddress(ctx context.Context, address string) (*treasury.Record, error) {
	model, err := dbGetByAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromTreasuryPoolModel(model), nil
}

// GetByVault implements treasury.Store.GetByVault
func (s *store) GetByVault(ctx context.Context, vault string) (*treasury.Record, error) {
	model, err := dbGetByVault(ctx, s.db, vault)
	if err != nil {
		return nil, err
	}

	return fromTreasuryPoolModel(model), nil
}

// GetAllByState implements treasury.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state treasury.TreasuryPoolState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*treasury.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	res := make([]*treasury.Record, len(models))
	for i, model := range models {
		res[i] = fromTreasuryPoolModel(model)
	}
	return res, nil
}

// SaveFunding implements treasury.Store.SaveFunding
func (s *store) SaveFunding(ctx context.Context, record *treasury.FundingHistoryRecord) error {
	model, err := toFundingModel(record)
	if err != nil {
		return err
	}

	if err := model.dbSave(ctx, s.db); err != nil {
		return err
	}

	res := fromFundingModel(model)
	res.CopyTo(record)

	return nil
}

// GetTotalAvailableFunds implements treasury.Store.GetTotalAvailableFunds
func (s *store) GetTotalAvailableFunds(ctx context.Context, vault string) (uint64, error) {
	return dbGetTotalAvailableFunds(ctx, s.db, vault)
}
