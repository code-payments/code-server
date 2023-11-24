package postgres

import (
	"context"
	"database/sql"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
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

// Count returns the total count of nonce accounts.
func (s *store) Count(ctx context.Context) (uint64, error) {
	return dbGetCount(ctx, s.db)
}

// Count returns the total count of nonce accounts by state
func (s *store) CountByState(ctx context.Context, state nonce.State) (uint64, error) {
	return dbGetCountByState(ctx, s.db, state)
}

// CountByStateAndPurpose returns the total count of nonce accounts in the provided
// state and use case
func (s *store) CountByStateAndPurpose(ctx context.Context, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	return dbGetCountByStateAndPurpose(ctx, s.db, state, purpose)
}

// Put saves nonce metadata to the store.
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

// Get finds the nonce record for a given address.
//
// Returns ErrNotFound if no record is found.
func (s *store) Get(ctx context.Context, address string) (*nonce.Record, error) {
	obj, err := dbGetNonce(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	return fromNonceModel(obj), nil
}

// GetAllByState returns nonce records in the store for a given
// confirmation state.
//
// Returns ErrNotFound if no records are found.
func (s *store) GetAllByState(ctx context.Context, state nonce.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*nonce.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	nonces := make([]*nonce.Record, len(models))
	for i, model := range models {
		nonces[i] = fromNonceModel(model)
	}

	return nonces, nil
}

// GetRandomAvailableByPurpose gets a random available nonce for a purpose.
//
// Returns ErrNotFound if no records are found.
func (s *store) GetRandomAvailableByPurpose(ctx context.Context, purpose nonce.Purpose) (*nonce.Record, error) {
	model, err := dbGetRandomAvailableByPurpose(ctx, s.db, purpose)
	if err != nil {
		return nil, err
	}
	return fromNonceModel(model), nil
}
