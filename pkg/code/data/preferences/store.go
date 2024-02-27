package preferences

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

var (
	ErrPreferencesNotFound = errors.New("preferences record not found")
)

type Store interface {
	// Save saves a preferences record
	Save(ctx context.Context, record *Record) error

	// Get gets a a preference record by a data container. ErrPreferencesNotFound
	// is returned if no record exists.
	Get(ctx context.Context, id *user.DataContainerID) (*Record, error)
}
