package messaging

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	ErrDuplicateMessageID = errors.New("duplicate message id")
)

type Record struct {
	Account   string
	MessageID uuid.UUID
	Message   []byte
}

// Store stores messages.
type Store interface {
	// Insert inserts a message into a specified bin.
	//
	// ErrDuplicateMessageID is returned if the message's ID already exists in the bin.
	Insert(ctx context.Context, record *Record) error

	// Delete deletes a message in the specified bin.
	//
	// Delete is idempotent.
	Delete(ctx context.Context, account string, messageID uuid.UUID) error

	// Get returns the messages in a bin.
	Get(ctx context.Context, account string) ([]*Record, error)
}

func (r *Record) Validate() error {
	if len(r.Account) == 0 {
		return errors.New("account is required")
	}

	var defaultUUID uuid.UUID
	if r.MessageID == defaultUUID {
		return errors.New("message id is required")
	}

	if len(r.Message) == 0 {
		return errors.New("message is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	copied := Record{
		Account: r.Account,
		Message: make([]byte, len(r.Message)),
	}

	copy(copied.MessageID[:], r.MessageID[:])
	copy(copied.Message, r.Message)

	return copied
}
