package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/paywall"
)

type store struct {
	mu      sync.Mutex
	records []*paywall.Record
	last    uint64
}

func New() paywall.Store {
	return &store{
		records: make([]*paywall.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*paywall.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

// Put implements paywall.Store.Put
func (s *store) Put(_ context.Context, data *paywall.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		return paywall.ErrPaywallExists
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

// GetByShortPath implements paywall.Store.GetByShortPath
func (s *store) GetByShortPath(_ context.Context, path string) (*paywall.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByShortPath(path)
	if item == nil {
		return nil, paywall.ErrPaywallNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) find(data *paywall.Record) *paywall.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.ShortPath == data.ShortPath {
			return item
		}
	}
	return nil
}

func (s *store) findByShortPath(path string) *paywall.Record {
	for _, item := range s.records {
		if item.ShortPath == path {
			return item
		}
	}
	return nil
}
