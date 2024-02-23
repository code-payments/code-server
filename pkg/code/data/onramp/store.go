package onramp

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrPurchaseAlreadyExists indicates an onramp purchase record already exists
	ErrPurchaseAlreadyExists = errors.New("onramp purchase record already exists")

	// ErrPurchaseNotFound indicates an onramp purchase record doesn't exists
	ErrPurchaseNotFound = errors.New("onramp purchase record not found")
)

type Store interface {
	// Put creates a new onramp purchase record
	Put(ctx context.Context, record *Record) error

	// Get gets an onramp purchase record by nonce
	Get(ctx context.Context, nonce uuid.UUID) (*Record, error)
}
