package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres twitter.Store
func New(db *sql.DB) twitter.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// SaveUser implements twitter.Store.SaveUser
func (s *store) SaveUser(ctx context.Context, record *twitter.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbSave(ctx, s.db)
	if err != nil {
		return err
	}

	res := fromModel(model)
	res.CopyTo(record)

	return nil
}

// GetUserByUsername implements twitter.Store.GetUserByUsername
func (s *store) GetUserByUsername(ctx context.Context, username string) (*twitter.Record, error) {
	model, err := dbGetUserByUsername(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetUserByTipAddress implements twitter.Store.GetUserByTipAddress
func (s *store) GetUserByTipAddress(ctx context.Context, tipAddress string) (*twitter.Record, error) {
	model, err := dbGetUserByTipAddress(ctx, s.db, tipAddress)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetStaleUsers implements twitter.Store.GetStaleUsers
func (s *store) GetStaleUsers(ctx context.Context, minAge time.Duration, limit int) ([]*twitter.Record, error) {
	models, err := dbGetStaleUsers(ctx, s.db, minAge, limit)
	if err != nil {
		return nil, err
	}

	res := make([]*twitter.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}

// MarkTweetAsProcessed implements twitter.Store.MarkTweetAsProcessed
func (s *store) MarkTweetAsProcessed(ctx context.Context, tweetId string) error {
	return dbMarkTweetAsProcessed(ctx, s.db, tweetId)
}

// IsTweetProcessed implements twitter.Store.IsTweetProcessed
func (s *store) IsTweetProcessed(ctx context.Context, tweetId string) (bool, error) {
	return dbIsTweetProcessed(ctx, s.db, tweetId)
}

// MarkNonceAsUsed implements twitter.Store.MarkNonceAsUsed
func (s *store) MarkNonceAsUsed(ctx context.Context, tweetId string, nonce uuid.UUID) error {
	return dbMarkNonceAsUsed(ctx, s.db, tweetId, nonce)
}
