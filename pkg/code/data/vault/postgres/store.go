package postgres

import (
	"context"
	"database/sql"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) vault.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Count returns the total count of keys.
func (s *store) Count(ctx context.Context) (uint64, error) {
	return dbGetCount(ctx, s.db)
}

// CountByState returns the total count of keys by state
func (s *store) CountByState(ctx context.Context, state vault.State) (uint64, error) {
	return dbGetCountByState(ctx, s.db, state)
}

// Save creates or updates vault metadata in the store.
func (s *store) Save(ctx context.Context, record *vault.Record) error {
	obj, err := toKeyModel(record)
	if err != nil {
		return err
	}

	ciphertext, err := vault.Encrypt(record.PrivateKey, record.PublicKey)
	if err != nil {
		return err
	}

	obj.PrivateKey = ciphertext
	err = obj.dbSave(ctx, s.db)
	if err != nil {
		return err
	}
	obj.PrivateKey = record.PrivateKey

	res := fromKeyModel(obj)
	res.CopyTo(record)

	return nil
}

// Get finds the vault record for a given pubkey.
func (s *store) Get(ctx context.Context, pubkey string) (*vault.Record, error) {
	obj, err := dbGetKey(ctx, s.db, pubkey)
	if err != nil {
		return nil, err
	}

	plaintext, err := vault.Decrypt(obj.PrivateKey, obj.PublicKey)
	if err != nil {
		return nil, err
	}
	obj.PrivateKey = plaintext

	return fromKeyModel(obj), nil
}

// GetAllByState returns all vault records for a given state.
//
// Returns ErrKeyNotFound if no records are found.
func (s *store) GetAllByState(ctx context.Context, state vault.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*vault.Record, error) {
	models, err := dbGetAllByState(ctx, s.db, state, cursor, limit, direction)
	if err != nil {
		return nil, err
	}

	keys := make([]*vault.Record, len(models))
	for i, model := range models {

		plaintext, err := vault.Decrypt(model.PrivateKey, model.PublicKey)
		if err != nil {
			return nil, err
		}
		model.PrivateKey = plaintext

		keys[i] = fromKeyModel(model)
	}

	return keys, nil
}
