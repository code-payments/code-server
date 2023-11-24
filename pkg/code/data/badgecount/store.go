package badgecount

import (
	"context"
	"errors"
)

var (
	ErrBadgeCountNotFound = errors.New("badge count not found")
)

type Store interface {
	// Add adds to the owner account's badge count
	Add(ctx context.Context, owner string, amount uint32) error

	// Reset resets the badge count for an owner account to zero
	Reset(ctx context.Context, owner string) error

	// Get gets a badge count record for an owner
	Get(ctx context.Context, owner string) (*Record, error)
}
