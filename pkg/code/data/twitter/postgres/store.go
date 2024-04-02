package postgres

import (
	"context"
	"database/sql"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
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

// GetUser implements twitter.Store.GetUser
func (s *store) GetUser(ctx context.Context, username string) (*twitter.Record, error) {
	model, err := dbGet(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// MarkTweetAsProcessed implements twitter.Store.MarkTweetAsProcessed
func (s *store) MarkTweetAsProcessed(ctx context.Context, tweetId string) error {
	return dbMarkTweetAsProcessed(ctx, s.db, tweetId)
}

// IsTweetProcessed implements twitter.Store.IsTweetProcessed
func (s *store) IsTweetProcessed(ctx context.Context, tweetId string) (bool, error) {
	return dbIsTweetProcessed(ctx, s.db, tweetId)
}
