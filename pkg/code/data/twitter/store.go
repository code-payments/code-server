package twitter

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound = errors.New("twitter user not found")
)

type Store interface {
	// SaveUser saves a Twitter user's information
	SaveUser(ctx context.Context, record *Record) error

	// GetUser gets a Twitter user's information
	GetUser(ctx context.Context, username string) (*Record, error)

	// GetStaleUsers gets user that have their last updated timestamp older than minAge
	GetStaleUsers(ctx context.Context, minAge time.Duration, limit int) ([]*Record, error)

	// MarkTweetAsProcessed marks a tweet as being processed
	MarkTweetAsProcessed(ctx context.Context, tweetId string) error

	// IsTweetProcessed returns whether a tweet is processed
	IsTweetProcessed(ctx context.Context, tweetId string) (bool, error)
}
