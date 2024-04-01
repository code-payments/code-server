package twitter

import (
	"context"
	"errors"
)

var (
	ErrUserNotFound = errors.New("twitter user not found")
)

type Store interface {
	// Save saves a Twitter user's information
	Save(ctx context.Context, record *Record) error

	// Get gets a Twitter user's information
	Get(ctx context.Context, username string) (*Record, error)
}
