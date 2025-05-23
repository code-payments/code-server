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

// Put implements rendezvous.Store.Put
func (s *store) Put(_ context.Context, data *rendezvous.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		if item.ExpiresAt.After(time.Now()) {
			return rendezvous.ErrExists
		}

		item.Address = data.Address
		item.ExpiresAt = data.ExpiresAt

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}

		cloned := data.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// ExtendExpiry implements rendezvous.Store.ExtendExpiry
func (s *store) ExtendExpiry(_ context.Context, key, address string, expiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByKeyAndAddress(key, address)
	if item == nil {
		return rendezvous.ErrNotFound
	}

	if item.ExpiresAt.Before(time.Now()) {
		return rendezvous.ErrNotFound
	}

	item.ExpiresAt = expiry

	return nil
}

// Delete implements rendezvous.Store.Delete
func (s *store) Delete(_ context.Context, key, address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.records {
		if item.Key == key && item.Address == address {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return nil
		}
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

	if item.ExpiresAt.Before(time.Now()) {
		return nil, rendezvous.ErrNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
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

func (s *store) findByKeyAndAddress(key, address string) *rendezvous.Record {
	for _, item := range s.records {
		if item.Key == key && item.Address == address {
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
