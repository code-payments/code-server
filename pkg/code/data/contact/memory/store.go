package memory

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/contact"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	mu              sync.RWMutex
	contactsByOwner map[string][]string
}

// New returns a new postgres backed contact.Store
func New() contact.Store {
	return &store{
		contactsByOwner: make(map[string][]string),
	}
}

// Add implements contact.Store.Add
func (s *store) Add(ctx context.Context, owner *user.DataContainerID, contact string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	contacts := s.contactsByOwner[owner.String()]
	for _, existing := range contacts {
		if existing == contact {
			return nil
		}
	}

	contacts = append(contacts, contact)
	s.contactsByOwner[owner.String()] = contacts

	return nil
}

// BatchAdd implements contact.Store.BatchAdd
func (s *store) BatchAdd(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	for _, contact := range contacts {
		err := s.Add(ctx, owner, contact)
		if err != nil {
			return err
		}
	}
	return nil
}

// Remove implements contact.Store.Remove
func (s *store) Remove(ctx context.Context, owner *user.DataContainerID, contact string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	contacts := s.contactsByOwner[owner.String()]
	for i, existing := range contacts {
		if existing == contact {
			s.contactsByOwner[owner.String()] = append(contacts[:i], contacts[i+1:]...)
			return nil
		}
	}

	return nil
}

// BatchRemove implements contact.Store.BatchRemove
func (s *store) BatchRemove(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	for _, contact := range contacts {
		err := s.Remove(ctx, owner, contact)
		if err != nil {
			return err
		}
	}
	return nil
}

// Get implements contact.Store.Get
func (s *store) Get(ctx context.Context, owner *user.DataContainerID, limit uint32, pageToken []byte) ([]string, []byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lowerBound uint64
	if len(pageToken) == 8 {
		lowerBound = binary.LittleEndian.Uint64(pageToken)
	} else if len(pageToken) != 0 {
		return nil, nil, errors.New("invalid page token bytes")
	}

	contacts, ok := s.contactsByOwner[owner.String()]
	if !ok {
		return nil, nil, nil
	}

	if len(contacts) <= int(lowerBound) {
		return nil, nil, nil
	}

	upperBound := lowerBound + uint64(limit)
	if int(upperBound) > len(contacts) {
		upperBound = uint64(len(contacts))
	}

	var nextPageToken []byte
	if len(contacts) > int(upperBound) {
		nextPageToken = make([]byte, 8)
		binary.LittleEndian.PutUint64(nextPageToken, upperBound)
	}

	return contacts[lowerBound:upperBound], nextPageToken, nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.contactsByOwner = make(map[string][]string)
}
