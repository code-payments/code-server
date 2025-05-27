package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) nonce.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s *store) Count(ctx context.Context, env nonce.Environment, instance string) (uint64, error) {
	return dbGetCount(ctx, s.db, env, instance)
}

func (s *store) CountByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State) (uint64, error) {
	return dbGetCountByState(ctx, s.db, env, instance, state)
}

func (s *store) CountByStateAndPurpose(ctx context.Context, env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	return dbGetCountByStateAndPurpose(ctx, s.db, env, instance, state, purpose)
}

func (s *store) Save(ctx context.Context, record *nonce.Record) error {
	obj, err := toNonceModel(record)
	if err != nil {
		return err
	}

	err = obj.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromNonceModel(obj)
	res.CopyTo(record)

	return nil
}

func (s *store) Get(ctx context.Context, address string) (*nonce.Record, error) {
	obj, err := dbGetNonce(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromNonceModel(obj), nil
}

func (s *store) GetAllByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*nonce.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, env, instance, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	nonces := make([]*nonce.Record, len(models))
	for i, model := range models {
		nonces[i] = fromNonceModel(model)
	}

	return nonces, nil
}

func (s *store) GetRandomAvailableByPurpose(ctx context.Context, env nonce.Environment, instance string, purpose nonce.Purpose) (*nonce.Record, error) {
	model, err := dbGetRandomAvailableByPurpose(ctx, s.db, env, instance, purpose)
	if err != nil {
		return nil, err
	}
	return fromNonceModel(model), nil
}

func (s *store) BatchClaimAvailableByPurpose(ctx context.Context, env nonce.Environment, instance string, purpose nonce.Purpose, limit int, nodeID string, minExpireAt, maxExpireAt time.Time) ([]*nonce.Record, error) {
	models, err := dbBatchClaimAvailableByPurpose(ctx, s.db, env, instance, purpose, limit, nodeID, minExpireAt, maxExpireAt)
	if err != nil {
		return nil, err
	}

	nonces := make([]*nonce.Record, len(models))
	for i, model := range models {
		nonces[i] = fromNonceModel(model)
	}

	return nonces, nil
}
