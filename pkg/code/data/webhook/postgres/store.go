package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed webhook.Store
func New(db *sql.DB) webhook.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements webhook.Store.Put
func (s *store) Put(ctx context.Context, record *webhook.Record) error {
	obj, err := toModel(record)
	if err != nil {
		return err
	}

	err = obj.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(obj)
	res.CopyTo(record)

	return nil
}

// Update implements webhook.Store.Update
func (s *store) Update(ctx context.Context, record *webhook.Record) error {
	obj, err := toModel(record)
	if err != nil {
		return err
	}

	err = obj.dbUpdate(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(obj)
	res.CopyTo(record)

	return nil
}

// Get implements webhook.Store.Get
func (s *store) Get(ctx context.Context, webhookId string) (*webhook.Record, error) {
	model, err := dbGetByWebhookId(ctx, s.db, webhookId)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// CountByState implements webhook.Store.CountByState
func (s *store) CountByState(ctx context.Context, state webhook.State) (uint64, error) {
	return dbCountByState(ctx, s.db, state)
}

// GetAllPendingReadyToSend implements webhook.Store.GetAllPendingReadyToSend
func (s *store) GetAllPendingReadyToSend(ctx context.Context, limit uint64) ([]*webhook.Record, error) {
	models, err := dbGetAllPendingReadyToSend(ctx, s.db, limit)
	if err != nil {
		return nil, err
	}

	var res []*webhook.Record
	for _, model := range models {
		res = append(res, fromModel(model))
	}
	return res, nil
}
