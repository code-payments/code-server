package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/action"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) action.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// PutAll implements action.store.PutAll
func (s *store) PutAll(ctx context.Context, records ...*action.Record) error {
	if len(records) == 0 {
		return errors.New("empty action set")
	}

	models := make([]*model, len(records))
	for i, record := range records {
		if record.Id > 0 {
			return action.ErrActionExists
		}

		model, err := toModel(record)
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
			return errors.New("unexpected count of action models returned")
		}

		// Don't assume postgres properly orders things
		updatedById := make(map[string]struct{})
		for _, model := range updated {
			key := fmt.Sprintf("%s:%d", model.Intent, model.ActionId)
			_, ok := updatedById[key]
			if ok {
				return errors.New("got a duplicate action model")
			}

			var found bool
			for _, record := range records {
				if record.Intent == model.Intent && uint32(model.ActionId) == record.ActionId {
					converted := fromModel(model)
					converted.CopyTo(record)

					found = true
				}
			}

			if !found {
				return errors.New("unexpected action model id")
			}
			updatedById[key] = struct{}{}
		}

		return nil
	})
}

// Update implements action.store.Update
func (s *store) Update(ctx context.Context, record *action.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	return model.dbUpdate(ctx, s.db)
}

// GetById implements action.store.GetById
func (s *store) GetById(ctx context.Context, intent string, actionId uint32) (*action.Record, error) {
	model, err := dbGetById(ctx, s.db, intent, actionId)
	if err != nil {
		return nil, err
	}

	return fromModel(model), nil
}

// GetAllByIntent implements action.store.GetAllByIntent
func (s *store) GetAllByIntent(ctx context.Context, intent string) ([]*action.Record, error) {
	models, err := dbGetAllByIntent(ctx, s.db, intent)
	if err != nil {
		return nil, err
	}

	records := make([]*action.Record, len(models))
	for i, model := range models {
		records[i] = fromModel(model)
	}
	return records, nil
}

// GetAllByAddress implements action.store.GetAllByAddress
func (s *store) GetAllByAddress(ctx context.Context, address string) ([]*action.Record, error) {
	models, err := dbGetAllByAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	records := make([]*action.Record, len(models))
	for i, model := range models {
		records[i] = fromModel(model)
	}
	return records, nil
}

// GetNetBalance implements action.store.GetNetBalance
func (s *store) GetNetBalance(ctx context.Context, account string) (int64, error) {
	return dbGetNetBalance(ctx, s.db, account)
}

// GetNetBalanceBatch implements action.store.GetNetBalanceBatch
func (s *store) GetNetBalanceBatch(ctx context.Context, accounts ...string) (map[string]int64, error) {
	return dbGetNetBalanceBatch(ctx, s.db, accounts...)
}

// GetGiftCardClaimedAction implements action.store.GetGiftCardClaimedAction
func (s *store) GetGiftCardClaimedAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	model, err := dbGetGiftCardClaimedAction(ctx, s.db, giftCardVault)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetGiftCardAutoReturnAction implements action.store.GetGiftCardAutoReturnAction
func (s *store) GetGiftCardAutoReturnAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	model, err := dbGetGiftCardAutoReturnAction(ctx, s.db, giftCardVault)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
