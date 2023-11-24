package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/badgecount"
)

type store struct {
	mu      sync.Mutex
	records []*badgecount.Record
	last    uint64
}

// New returns a new in memory badgecount.Store
func New() badgecount.Store {
	return &store{}
}

// Add implements badgecount.Store.Add
func (s *store) Add(_ context.Context, owner string, amount uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	item := s.findByOwner(owner)
	if item != nil {
		item.BadgeCount += amount
		item.LastUpdatedAt = time.Now()
	} else {
		s.records = append(s.records, &badgecount.Record{
			Id: s.last,

			Owner:      owner,
			BadgeCount: amount,

			LastUpdatedAt: time.Now(),
			CreatedAt:     time.Now(),
		})
	}

	return nil
}

// Reset implements badgecount.Store.Reset
func (s *store) Reset(_ context.Context, owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByOwner(owner)
	if item != nil {
		s.last++
		item.BadgeCount = 0
		item.LastUpdatedAt = time.Now()
	}

	return nil
}

// Get implements badgecount.Store.Get
func (s *store) Get(_ context.Context, owner string) (*badgecount.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByOwner(owner)
	if item == nil {
		return nil, badgecount.ErrBadgeCountNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) findByOwner(owner string) *badgecount.Record {
	for _, item := range s.records {
		if item.Owner == owner {
			return item
		}
	}
	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
