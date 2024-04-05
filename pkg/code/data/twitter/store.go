package twitter

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound        = errors.New("twitter user not found")
	ErrDuplicateTipAddress = errors.New("duplicate tip address")
)

type Store interface {
	// SaveUser saves a Twitter user's information
	SaveUser(ctx context.Context, record *Record) error

	// GetUserByUsername gets a Twitter user's information by the username
	GetUserByUsername(ctx context.Context, username string) (*Record, error)

	// GetUserByTipAddress gets a Twitter user's information by the tip address
	GetUserByTipAddress(ctx context.Context, tipAddress string) (*Record, error)

	// GetStaleUsers gets user that have their last updated timestamp older than minAge
	GetStaleUsers(ctx context.Context, minAge time.Duration, limit int) ([]*Record, error)

	// MarkTweetAsProcessed marks a tweet as being processed
	MarkTweetAsProcessed(ctx context.Context, tweetId string) error

	// IsTweetProcessed returns whether a tweet is processed
	IsTweetProcessed(ctx context.Context, tweetId string) (bool, error)
}
