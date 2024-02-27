package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	mu      sync.Mutex
	records []*preferences.Record
	last    uint64
}

// New returns a new in memory preferences.Store
func New() preferences.Store {
	return &store{}
}

// Save saves a preferences record
func (s *store) Save(_ context.Context, record *preferences.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	if item := s.find(record); item != nil {
		record.LastUpdatedAt = time.Now()

		item.Locale = record.Locale
		item.LastUpdatedAt = record.LastUpdatedAt
	} else {
		record.Id = s.last
		record.LastUpdatedAt = time.Now()

		cloned := record.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// Get gets a a preference record by a data container
func (s *store) Get(_ context.Context, id *user.DataContainerID) (*preferences.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByDataContainer(id)
	if item == nil {
		return nil, preferences.ErrPreferencesNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) find(data *preferences.Record) *preferences.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.DataContainerId.String() == data.DataContainerId.String() {
			return item
		}
	}
	return nil
}

func (s *store) findByDataContainer(id *user.DataContainerID) *preferences.Record {
	for _, item := range s.records {
		if item.DataContainerId.String() == id.String() {
			return item
		}
	}
	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.last = 0
	s.records = nil
}
