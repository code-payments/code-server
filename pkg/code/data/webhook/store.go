package webhook

import (
	"context"

	"github.com/pkg/errors"
)

var (
	ErrNotFound      = errors.New("webhook record not found")
	ErrAlreadyExists = errors.New("webhook record already exists")
)

type Store interface {
	// Put creates a webhook record
	//
	// Returns ErrAlreadyExists if a record already exists.
	Put(ctx context.Context, record *Record) error

	// Update updates a webhook record
	//
	// Returns ErrNotFound if no record exists.
	Update(ctx context.Context, record *Record) error

	// Get finds the webhook record for a given webhook ID
	//
	// Returns ErrNotFound if no record is found.
	Get(ctx context.Context, webhookId string) (*Record, error)

	// CountByState counts all webhook records in a provided state
	CountByState(ctx context.Context, state State) (uint64, error)

	// GetAllPendingReadyToSend gets all webhook records in the pending state
	// that have an attempt scheduled to be sent.
	//
	// Returns ErrNotFound if no record is found.
	//
	// Note: No traditional pagination since it's expected the next attempt
	//       timestamp is updated or the state transitions to a terminal value.
	GetAllPendingReadyToSend(ctx context.Context, limit uint64) ([]*Record, error)
}
