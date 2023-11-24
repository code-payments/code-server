package event

import (
	"context"
	"errors"
)

var (
	ErrEventNotFound = errors.New("event record not found")
)

type Store interface {
	// Save creates or updates an event record. For updates, only fields that can
	// be reasonably changed or provided at a later time are supported.
	Save(ctx context.Context, record *Record) error

	// Get gets an event record by its event ID
	Get(ctx context.Context, id string) (*Record, error)

	// todo: Various other methods that can help us with product or spam tracking
}
