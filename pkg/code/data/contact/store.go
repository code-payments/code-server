package contact

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

type Store interface {
	// Add adds the contact to the owner's contact list. This call is idempotent
	// and will not fail on duplicate insertion.
	Add(ctx context.Context, owner *user.DataContainerID, contact string) error

	// BatchAdd adds a batch of contacts to the owner's contact list. This call is
	// idempotent and will not fail on duplicate insertion.
	BatchAdd(ctx context.Context, owner *user.DataContainerID, contacts []string) error

	// Remove removes the contact to the owner's contact list. This call is
	// idempotent and will not fail on deletion of a non-existant entry.
	Remove(ctx context.Context, owner *user.DataContainerID, contact string) error

	// BatchDelete removes a batch of contacts to the owner's contact list. This
	// call is idempotent and will not fail on duplicate insertion.
	BatchRemove(ctx context.Context, owner *user.DataContainerID, contacts []string) error

	// Get gets a page of contacts from an owner's contact list.
	Get(ctx context.Context, owner *user.DataContainerID, limit uint32, pageToken []byte) (contacts []string, nextPageToken []byte, err error)
}
