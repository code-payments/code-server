package balance

import (
	"context"
	"errors"
)

var (
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	ErrStaleCheckpoint = errors.New("checkpoint is stale")
)

type Store interface {
	// SaveCheckpoint saves a balance at a checkpoint. ErrStaleCheckpoint is returned
	// if the checkpoint is outdated
	SaveCheckpoint(ctx context.Context, record *Record) error

	// GetCheckpoint gets a balance checkpoint for a given account. ErrCheckpointNotFound
	// is returend if no DB record exists.
	GetCheckpoint(ctx context.Context, account string) (*Record, error)
}
