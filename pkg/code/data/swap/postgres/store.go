package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) swap.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s *store) Save(ctx context.Context, record *swap.Record) error {
	obj, err := toModel(record)
	if err != nil {
		return err
	}

	err = obj.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(obj)
	res.CopyTo(record)

	return nil
}

func (s *store) GetById(ctx context.Context, id string) (*swap.Record, error) {
	obj, err := dbGetById(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return fromModel(obj), nil
}

func (s *store) GetAllByOwnerAndState(ctx context.Context, owner string, state swap.State) ([]*swap.Record, error) {
	models, err := dbGetAllByOwnerAndState(ctx, s.db, owner, state)
	if err != nil {
		return nil, err
	}

	records := make([]*swap.Record, len(models))
	for i, model := range models {
		records[i] = fromModel(model)
	}
	return records, nil
}

func (s *store) GetAllByState(ctx context.Context, state swap.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*swap.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	res := make([]*swap.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}
