package push

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

var (
	ErrTokenExists   = errors.New("push token already exists")
	ErrTokenNotFound = errors.New("push token not found for data container")
)

type Store interface {
	// Put creates a new push token record
	Put(ctx context.Context, record *Record) error

	// MarkAsInvalid marks all entries with the push token as invalid
	MarkAsInvalid(ctx context.Context, pushToken string) error

	// Delete deletes all entries with the push token
	Delete(ctx context.Context, pushToken string) error

	// GetAllValidByDataContainer gets all valid push token records for a given
	// data container.
	GetAllValidByDataContainer(ctx context.Context, id *user.DataContainerID) ([]*Record, error)
}
