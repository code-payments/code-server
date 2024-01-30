package balance

import (
	"context"
	"errors"
)

var (
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	ErrStaleCheckpoint = errors.New("checkpoint is stale")
)

// todo: comments
type Store interface {
	SaveCheckpoint(ctx context.Context, record *Record) error

	GetCheckpoint(ctx context.Context, account string) (*Record, error)
}
