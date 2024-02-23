package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) paymentrequest.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements paymentrequest.Store.Put
func (s *store) Put(ctx context.Context, record *paymentrequest.Record) error {
	m, err := toRequestModel(record)
	if err != nil {
		return err
	}

	err = m.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromRequestModel(m)
	res.CopyTo(record)

	return nil
}

// Get implements paymentrequest.Store.Get
func (s *store) Get(ctx context.Context, intentId string) (*paymentrequest.Record, error) {
	m, err := dbGet(ctx, s.db, intentId)
	if err != nil {
		return nil, err
	}
	return fromRequestModel(m), nil
}
