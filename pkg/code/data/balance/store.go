package balance

import (
	"context"
	"errors"
)

var (
	ErrStaleCachedBalanceVersion = errors.New("cached balance version is stale")

	ErrAccountClosed = errors.New("account open state is stale")

	ErrCheckpointNotFound = errors.New("checkpoint not found")
	ErrStaleCheckpoint    = errors.New("checkpoint is stale")
)

type Store interface {
	// GetCachedVersion gets the current cached balance version, which can be used
	// for optimistic locking cached balances for operations with outgoing transfers.
	GetCachedVersion(ctx context.Context, account string) (uint64, error)

	// AdvanceCachedVersion advances an account's cached balance version.
	//
	// ErrStaleCachedBalanceVersion is returned if the currentVersion is out of date.
	AdvanceCachedVersion(ctx context.Context, account string, currentVersion uint64) error

	// CheckNotClosed checks whether an account is closed under a lock to guarantee
	// payments to a closeable destination with cached balances are made to an open
	// account.
	//
	// ErrAccountClosed is returned if the account has been closed.
	CheckNotClosed(ctx context.Context, account string) error

	// MarkAsClosed marks an account as being closed and unable to receive payments
	// as a destination.
	MarkAsClosed(ctx context.Context, account string) error

	// SaveExternalCheckpoint saves an external balance at a checkpoint.
	//
	// ErrStaleCheckpoint is returned if the checkpoint is outdated
	SaveExternalCheckpoint(ctx context.Context, record *ExternalCheckpointRecord) error

	// GetExternalCheckpoint gets an exeternal balance checkpoint for a
	// given account.
	//
	// ErrCheckpointNotFound is returend if no DB record exists.
	GetExternalCheckpoint(ctx context.Context, account string) (*ExternalCheckpointRecord, error)
}
