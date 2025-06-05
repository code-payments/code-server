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
	// SaveExternalCheckpoint saves an external balance at a checkpoint.
	// ErrStaleCheckpoint is returned if the checkpoint is outdated
	SaveExternalCheckpoint(ctx context.Context, record *ExternalCheckpointRecord) error

	// GetExternalCheckpoint gets an exeternal balance checkpoint for a
	// given account. ErrCheckpointNotFound is returend if no DB record
	// exists.
	GetExternalCheckpoint(ctx context.Context, account string) (*ExternalCheckpointRecord, error)
}
