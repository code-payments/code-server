package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
)

type store struct {
	mu      sync.Mutex
	last    uint64
	records []*rendezvous.Record
}

// New returns a new in memory rendezvous.Store
func New() rendezvous.Store {
	return &store{}
}

// Save implements rendezvous.Store.Save
func (s *store) Save(_ context.Context, data *rendezvous.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.Location = data.Location
		item.LastUpdatedAt = time.Now()

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		data.CreatedAt = time.Now()
		data.LastUpdatedAt = time.Now()

		cloned := data.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// Get implements rendezvous.Store.Get
func (s *store) Get(_ context.Context, key string) (*rendezvous.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByKey(key)
	if item == nil {
		return nil, rendezvous.ErrNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// Delete implements rendezvous.Store.Delete
func (s *store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.records {
		if item.Key == key {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return nil
		}
	}

	return nil
}

func (s *store) find(data *rendezvous.Record) *rendezvous.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.Key == data.Key {
			return item
		}
	}

	return nil
}

func (s *store) findByKey(key string) *rendezvous.Record {
	for _, item := range s.records {
		if item.Key == key {
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
