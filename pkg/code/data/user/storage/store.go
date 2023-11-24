package storage

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

var (
	// ErrNotFound is returned when a data container being fetched does not exist.
	ErrNotFound = errors.New("data container not found")

	// ErrAlreadyExists is returned when a new container being added already
	// exists.
	ErrAlreadyExists = errors.New("data container already exists")
)

// DataContainer represents a logical boundary that separates and isolates a copy
// of a user's data. The owner account acts as the owner of the copy of data as
// both a form of authentication (via signing of the owner account private key)
// and authorization (by exact match of an authenticated owner account address
// linked to a container). The ID must be used as part of the key for user data
// systems.
type Record struct {
	ID                  *user.DataContainerID
	OwnerAccount        string
	IdentifyingFeatures *user.IdentifyingFeatures
	CreatedAt           time.Time
}

// Store manages copies of a user's data using logical containers.
type Store interface {
	// Put creates a new data container.
	Put(ctx context.Context, record *Record) error

	// GetByID gets a data container by its ID.
	GetByID(ctx context.Context, id *user.DataContainerID) (*Record, error)

	// GetByFeatures gets a data container that matches the set of features.
	GetByFeatures(ctx context.Context, ownerAccount string, features *user.IdentifyingFeatures) (*Record, error)
}

// Validate validates a Record
func (r *Record) Validate() error {
	if r == nil {
		return errors.New("record is nil")
	}

	if err := r.ID.Validate(); err != nil {
		return err
	}

	if len(r.OwnerAccount) == 0 {
		return errors.New("owner account is empty")
	}

	if err := r.IdentifyingFeatures.Validate(); err != nil {
		return err
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	return nil
}
