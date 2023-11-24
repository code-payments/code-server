package paywall

import (
	"context"
	"errors"
)

var (
	ErrPaywallNotFound = errors.New("paywall record not found")
	ErrPaywallExists   = errors.New("paywall record already exists")
)

type Store interface {
	Put(ctx context.Context, record *Record) error

	GetByShortPath(ctx context.Context, path string) (*Record, error)
}
