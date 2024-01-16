package paymentrequest

// todo: refactor this package to a generic "request" model, similar to intent

import (
	"context"
	"errors"
)

var (
	ErrPaymentRequestAlreadyExists = errors.New("payment request record already exists")
	ErrPaymentRequestNotFound      = errors.New("no payment request records could be found")
)

type Store interface {
	// Put creates a new payment request record
	Put(ctx context.Context, record *Record) error

	// Get gets a paymen request record by its intent ID
	Get(ctx context.Context, intentId string) (*Record, error)
}
