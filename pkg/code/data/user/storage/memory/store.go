package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"
)

type store struct {
	mu                       sync.RWMutex
	dataContainersByID       map[string]*user_storage.Record
	dataContainersByFeatures map[string]*user_storage.Record
}

// New returns an in memory user_storage.Store
func New() user_storage.Store {
	return &store{
		dataContainersByID:       make(map[string]*user_storage.Record),
		dataContainersByFeatures: make(map[string]*user_storage.Record),
	}
}

// Put implements user_storage.Store.Put
func (s *store) Put(ctx context.Context, container *user_storage.Record) error {
	if err := container.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.dataContainersByID[container.ID.String()]
	if ok {
		return user_storage.ErrAlreadyExists
	}

	key := getFeaturesKey(container.OwnerAccount, container.IdentifyingFeatures)
	_, ok = s.dataContainersByFeatures[key]
	if ok {
		return user_storage.ErrAlreadyExists
	}

	copy := &user_storage.Record{
		ID:           container.ID,
		OwnerAccount: container.OwnerAccount,
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: container.IdentifyingFeatures.PhoneNumber,
		},
		CreatedAt: container.CreatedAt,
	}

	s.dataContainersByID[container.ID.String()] = copy
	s.dataContainersByFeatures[key] = copy

	return nil
}

// GetByID implements user_storage.Store.GetByID
func (s *store) GetByID(ctx context.Context, id *user.DataContainerID) (*user_storage.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.dataContainersByID[id.String()]
	if !ok {
		return nil, user_storage.ErrNotFound
	}

	return record, nil
}

// GetByFeatures implements user_storage.Store.GetByFeatures
func (s *store) GetByFeatures(ctx context.Context, ownerAccount string, features *user.IdentifyingFeatures) (*user_storage.Record, error) {
	if err := features.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := getFeaturesKey(ownerAccount, features)
	record, ok := s.dataContainersByFeatures[key]
	if !ok {
		return nil, user_storage.ErrNotFound
	}

	return record, nil
}

func getFeaturesKey(ownerAccount string, features *user.IdentifyingFeatures) string {
	return fmt.Sprintf("%s:%s", ownerAccount, *features.PhoneNumber)
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dataContainersByID = make(map[string]*user_storage.Record)
	s.dataContainersByFeatures = make(map[string]*user_storage.Record)
}
