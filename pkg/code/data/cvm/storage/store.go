package storage

import (
	"context"
	"errors"
)

var (
	ErrAddressAlreadyReserved = errors.New("virtual account address already in storage")
	ErrAlreadyInitialized     = errors.New("storage account already initalized")
	ErrInvalidInitialCapacity = errors.New("available capacity must be maximum when initializing storage")
	ErrNoFreeStorage          = errors.New("no available free storage")
	ErrNotFound               = errors.New("no storage accounts found")
)

// Store implements a basic construct for managing compression storage.
//
// Note: A lock outside this implementation is required to resolve any races.
type Store interface {
	// Initializes a VM storage account for management
	InitializeStorage(ctx context.Context, record *Record) error

	// FindAnyWithAvailableCapacity finds a VM storage account with minimum available capcity
	FindAnyWithAvailableCapacity(ctx context.Context, vm string, purpose Purpose, minCapacity uint64) (*Record, error)

	// ReserveStorage reserves a piece of storage in a VM for the virtual account address
	ReserveStorage(ctx context.Context, vm string, purpose Purpose, address string) (string, error)
}
