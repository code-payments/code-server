package login

import (
	"context"
	"errors"
)

var (
	ErrLoginNotFound = errors.New("login not found")
)

type Store interface {
	// Save saves a multi login record in a single DB tx
	Save(ctx context.Context, record *MultiRecord) error

	// GetAllByInstallId gets a multi login record for an app install. If no
	// logins are detected, ErrLoginNotFound is returned.
	GetAllByInstallId(ctx context.Context, appInstallId string) (*MultiRecord, error)

	// GetLatestByOwner gets the latest login record for an owner. If no
	// login is detected, ErrLoginNotFound is returned.
	GetLatestByOwner(ctx context.Context, owner string) (*Record, error)
}
