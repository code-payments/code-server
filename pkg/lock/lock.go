package lock

import (
	"context"
)

// Manager creates and manages locks. Locks produced for a given name
// are re-entrant per Manager. This allows for implementations to
// operate more efficiently (and safely).
//
// As locks are re-entrant per manager, it is strongly advised to use
// a sync.Mutex (or some other state management mechanism) to coordinate
// local concurrency.
type Manager interface {
	// Create creates an unlocked DistributedLock for a specific key.
	Create(ctx context.Context, name string) (DistributedLock, error)
}

// DistributedLock is a handle to a distributed lock that spans across multiple
// processes. Two DistributedLock's for the same name produced by the same Manager
// are re-entrant. See Manager for details.
type DistributedLock interface {
	// Acquire attempts to acquire the lock, blocking until the lock has been
	// successfully acquired.
	//
	// The returned channel is a channel that will be closed when the lock is lost.
	// The lock can be lost when the context is cancelled, Unlock() is called, or
	// the underlying implementation detects that the lock _might_ have been lost.
	Acquire(ctx context.Context) (<-chan struct{}, error)

	// Unlock unlocks the lock, if the lock is held.
	//
	// Unlock is idempotent.
	Unlock(ctx context.Context) error

	// IsLocked returns whether the lock is held by the process/manager.
	IsLocked() bool
}
