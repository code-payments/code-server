package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed push.Store
func New(db *sql.DB) push.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements push.Store.Put
func (s *store) Put(ctx context.Context, record *push.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res, err := fromModel(model)
	if err != nil {
		return err
	}
	res.CopyTo(record)

	return nil
}

// MarkAsInvalid implements push.Store.MarkAsInvalid
func (s *store) MarkAsInvalid(ctx context.Context, pushToken string) error {
	return dbMarkAsInvalid(ctx, s.db, pushToken)
}

// Delete implements push.Store.Delete
func (s *store) Delete(ctx context.Context, pushToken string) error {
	return dbDelete(ctx, s.db, pushToken)
}

// GetAllValidByDataContainer implements push.Store.GetAllValidByDataContainer
func (s *store) GetAllValidByDataContainer(ctx context.Context, id *user.DataContainerID) ([]*push.Record, error) {
	models, err := dbGetAllValidByDataContainer(ctx, s.db, id)
	if err != nil {
		return nil, err
	}

	res := make([]*push.Record, len(models))
	for i, model := range models {
		res[i], err = fromModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}
