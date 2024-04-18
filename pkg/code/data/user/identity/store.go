package identity

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

var (
	// ErrNotFound is returned when a user being fetched does not exist.
	ErrNotFound = errors.New("user not found")

	// ErrAlreadyExists is returned when a new user record is being added already
	// exists.
	ErrAlreadyExists = errors.New("user already exists")
)

// User is the highest order of a form of identity.
type Record struct {
	ID          *user.Id
	View        *user.View
	IsStaffUser bool
	IsBanned    bool
	CreatedAt   time.Time
}

// Store manages a user's identity.
type Store interface {
	// Put creates a new user.
	Put(ctx context.Context, record *Record) error

	// GetByID fetches a user by its ID.
	GetByID(ctx context.Context, id *user.Id) (*Record, error)

	// GetByView fetches a user by a view.
	GetByView(ctx context.Context, view *user.View) (*Record, error)
}

// Validate validates a Record
func (r *Record) Validate() error {
	if r == nil {
		return errors.New("record is nil")
	}

	if err := r.ID.Validate(); err != nil {
		return err
	}

	if err := r.View.Validate(); err != nil {
		return err
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	return nil
}
