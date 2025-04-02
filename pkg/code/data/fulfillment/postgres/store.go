package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) fulfillment.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Count implements fulfillment.Store.Count
func (s *store) Count(ctx context.Context) (uint64, error) {
	return dbGetCount(ctx, s.db)
}

// CountByState implements fulfillment.Store.CountByState
func (s *store) CountByState(ctx context.Context, state fulfillment.State) (uint64, error) {
	return dbGetCountByState(ctx, s.db, state)
}

// CountByStateGroupedByType implements fulfillment.Store.CountByStateGroupedByType
func (s *store) CountByStateGroupedByType(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	return dbGetCountByStateGroupedByType(ctx, s.db, state)
}

// CountForMetrics implements fulfillment.Store.CountForMetrics
func (s *store) CountForMetrics(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	return dbGetCountForMetrics(ctx, s.db, state)
}

// CountByStateAndAddress implements fulfillment.Store.CountByStateAndAddress
func (s *store) CountByStateAndAddress(ctx context.Context, state fulfillment.State, address string) (uint64, error) {
	return dbGetCountByStateAndAddress(ctx, s.db, state, address)
}

// CountByTypeStateAndAddress implements fulfillment.Store.CountByTypeStateAndAddress
func (s *store) CountByTypeStateAndAddress(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	return dbGetCountByTypeStateAndAddress(ctx, s.db, fulfillmentType, state, address)
}

// CountByTypeStateAndAddressAsSource implements fulfillment.Store.CountByTypeStateAndAddressAsSource
func (s *store) CountByTypeStateAndAddressAsSource(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	return dbGetCountByTypeStateAndAddressAsSource(ctx, s.db, fulfillmentType, state, address)
}

// CountByIntentAndState implements fulfillment.Store.CountByIntentAndState
func (s *store) CountByIntentAndState(ctx context.Context, intent string, state fulfillment.State) (uint64, error) {
	return dbGetCountByIntentAndState(ctx, s.db, intent, state)
}

// CountByIntent implements fulfillment.Store.CountByIntent
func (s *store) CountByIntent(ctx context.Context, intent string) (uint64, error) {
	return dbGetCountByIntent(ctx, s.db, intent)
}

// CountByTypeActionAndState implements fulfillment.Store.CountByTypeActionAndState
func (s *store) CountByTypeActionAndState(ctx context.Context, intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) (uint64, error) {
	return dbGetCountByTypeActionAndState(ctx, s.db, intentId, actionId, fulfillmentType, state)
}

// CountPendingByType implements fulfillment.Store.CountPendingByType
func (s *store) CountPendingByType(ctx context.Context) (map[fulfillment.Type]uint64, error) {
	return dbGetCountPendingByType(ctx, s.db)
}

// PutAll implements fulfillment.Store.PutAll
func (s *store) PutAll(ctx context.Context, records ...*fulfillment.Record) error {
	models := make([]*fulfillmentModel, len(records))
	for i, record := range records {
		model, err := toFulfillmentModel(record)
		if err != nil {
			return err
		}

		models[i] = model
	}

	return pgutil.ExecuteInTx(ctx, s.db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		updated, err := dbPutAllInTx(ctx, tx, models)
		if err != nil {
			return err
		}

		if len(updated) != len(records) {
			return errors.New("unexpected count of fulfillment models returned")
		}

		for i, model := range updated {
			converted := fromFulfillmentModel(model)
			converted.CopyTo(records[i])
		}

		return nil
	})
}

// Update implements fulfillment.Store.Update
func (s *store) Update(ctx context.Context, record *fulfillment.Record) error {
	obj, err := toFulfillmentModel(record)
	if err != nil {
		return err
	}

	err = obj.dbUpdate(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromFulfillmentModel(obj)
	res.CopyTo(record)

	return nil
}

// MarkAsActivelyScheduled implements fulfillment.Store.MarkAsActivelyScheduled
func (s *store) MarkAsActivelyScheduled(ctx context.Context, id uint64) error {
	return dbMarkAsActivelyScheduled(ctx, s.db, id)
}

// GetById implements fulfillment.Store.GetById
func (s *store) GetById(ctx context.Context, id uint64) (*fulfillment.Record, error) {
	obj, err := dbGetById(ctx, s.db, id)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(obj), nil
}

// GetBySignature implements fulfillment.Store.GetBySignature
func (s *store) GetBySignature(ctx context.Context, signature string) (*fulfillment.Record, error) {
	obj, err := dbGetBySignature(ctx, s.db, signature)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(obj), nil
}

// GetByVirtualSignature implements fulfillment.Store.GetByVirtualSignature
func (s *store) GetByVirtualSignature(ctx context.Context, signature string) (*fulfillment.Record, error) {
	obj, err := dbGetByVirtualSignature(ctx, s.db, signature)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(obj), nil
}

// GetAllByState implements fulfillment.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state fulfillment.State, includeDisabledActiveScheduling bool, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*fulfillment.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, includeDisabledActiveScheduling, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	fulfillments := make([]*fulfillment.Record, len(models))
	for i, model := range models {
		fulfillments[i] = fromFulfillmentModel(model)
	}

	return fulfillments, nil
}

// GetAllByIntent implements fulfillment.Store.GetAllByIntent
func (s *store) GetAllByIntent(ctx context.Context, intent string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*fulfillment.Record, error) {
	models, err := dbGetAllByIntent(ctx, s.db, intent, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	fulfillments := make([]*fulfillment.Record, len(models))
	for i, model := range models {
		fulfillments[i] = fromFulfillmentModel(model)
	}

	return fulfillments, nil
}

// GetAllByAction implements fulfillment.Store.GetAllByAction
func (s *store) GetAllByAction(ctx context.Context, intent string, actionId uint32) ([]*fulfillment.Record, error) {
	models, err := dbGetAllByAction(ctx, s.db, intent, actionId)
	if err != nil {
		return nil, err
	}

	fulfillments := make([]*fulfillment.Record, len(models))
	for i, model := range models {
		fulfillments[i] = fromFulfillmentModel(model)
	}

	return fulfillments, nil
}

// GetAllByTypeAndAction implements fulfillment.Store.GetAllByTypeAndAction
func (s *store) GetAllByTypeAndAction(ctx context.Context, fulfillmentType fulfillment.Type, intentId string, actionId uint32) ([]*fulfillment.Record, error) {
	models, err := dbGetAllByTypeAndAction(ctx, s.db, fulfillmentType, intentId, actionId)
	if err != nil {
		return nil, err
	}

	fulfillments := make([]*fulfillment.Record, len(models))
	for i, model := range models {
		fulfillments[i] = fromFulfillmentModel(model)
	}

	return fulfillments, nil
}

// GetFirstSchedulableByAddressAsSource implements fulfillment.Store.GetFirstSchedulableByAddressAsSource
func (s *store) GetFirstSchedulableByAddressAsSource(ctx context.Context, address string) (*fulfillment.Record, error) {
	model, err := dbGetFirstSchedulableByAddressAsSource(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(model), nil
}

// GetFirstSchedulableByAddressAsDestination implements fulfillment.Store.GetFirstSchedulableByAddressAsDestination
func (s *store) GetFirstSchedulableByAddressAsDestination(ctx context.Context, address string) (*fulfillment.Record, error) {
	model, err := dbGetFirstSchedulableByAddressAsDestination(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(model), nil
}

// GetFirstSchedulableByType implements fulfillment.Store.GetFirstSchedulableByType
func (s *store) GetFirstSchedulableByType(ctx context.Context, fulfillmentType fulfillment.Type) (*fulfillment.Record, error) {
	model, err := dbGetFirstSchedulableByType(ctx, s.db, fulfillmentType)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(model), nil
}

// GetNextSchedulableByAddress implements fulfillment.Store.GetNextSchedulableByAddress
func (s *store) GetNextSchedulableByAddress(ctx context.Context, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*fulfillment.Record, error) {
	model, err := dbGetNextSchedulableByAddress(ctx, s.db, address, intentOrderingIndex, actionOrderingIndex, fulfillmentOrderingIndex)
	if err != nil {
		return nil, err
	}

	return fromFulfillmentModel(model), nil
}
