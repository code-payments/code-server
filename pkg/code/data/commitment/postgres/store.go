package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	db *sqlx.DB
}

// New returns a new in memory commitment.Store
func New(db *sql.DB) commitment.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements commitment.Store.Save
func (s *store) Save(ctx context.Context, record *commitment.Record) error {
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

// GetByAddress implements commitment.Store.GetByAddress
func (s *store) GetByAddress(ctx context.Context, address string) (*commitment.Record, error) {
	model, err := dbGetByAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetByAction implements commitment.Store.GetByAction
func (s *store) GetByAction(ctx context.Context, intentId string, actionId uint32) (*commitment.Record, error) {
	model, err := dbGetByAction(ctx, s.db, intentId, actionId)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetAllByState implements commitment.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state commitment.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*commitment.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	res := make([]*commitment.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}

// GetUpgradeableByOwner implements commitment.Store.GetUpgradeableByOwner
func (s *store) GetUpgradeableByOwner(ctx context.Context, owner string, limit uint64) ([]*commitment.Record, error) {
	models, err := dbGetUpgradeableByOwner(ctx, s.db, owner, limit)
	if err != nil {
		return nil, err
	}

	res := make([]*commitment.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}

// GetUsedTreasuryPoolDeficit implements commitment.Store.GetUsedTreasuryPoolDeficit
func (s *store) GetUsedTreasuryPoolDeficit(ctx context.Context, pool string) (uint64, error) {
	return dbGetUsedTreasuryPoolDeficit(ctx, s.db, pool)
}

// GetTotalTreasuryPoolDeficit implements commitment.Store.GetTotalTreasuryPoolDeficit
func (s *store) GetTotalTreasuryPoolDeficit(ctx context.Context, pool string) (uint64, error) {
	return dbGetTotalTreasuryPoolDeficit(ctx, s.db, pool)
}

// CountByState implements commitment.Store.CountByState
func (s *store) CountByState(ctx context.Context, state commitment.State) (uint64, error) {
	return dbCountByState(ctx, s.db, state)
}

// CountPendingRepaymentsDivertedToCommitment implements commitment.Store.CountPendingRepaymentsDivertedToCommitment
func (s *store) CountPendingRepaymentsDivertedToCommitment(ctx context.Context, address string) (uint64, error) {
	return dbCountPendingRepaymentsDivertedToCommitment(ctx, s.db, address)
}
