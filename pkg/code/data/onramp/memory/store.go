package memory

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/code-payments/code-server/pkg/code/data/onramp"
)

type store struct {
	mu      sync.Mutex
	records []*onramp.Record
	last    uint64
}

// New returns a new in memory onramp.Store
func New() onramp.Store {
	return &store{}
}

// Put implements onramp.Store.Put
func (s *store) Put(_ context.Context, data *onramp.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		return onramp.ErrPurchaseAlreadyExists
	}

	if data.Id == 0 {
		data.Id = s.last
	}
	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now()
	}
	c := data.Clone()
	s.records = append(s.records, &c)

	return nil
}

// Get implements onramp.Store.Get
func (s *store) Get(_ context.Context, nonce uuid.UUID) (*onramp.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByNonce(nonce); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}
	return nil, onramp.ErrPurchaseNotFound
}

func (s *store) find(data *onramp.Record) *onramp.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if bytes.Equal(data.Nonce[:], item.Nonce[:]) {
			return item
		}
	}
	return nil
}

func (s *store) findByNonce(nonce uuid.UUID) *onramp.Record {
	for _, item := range s.records {
		if bytes.Equal(nonce[:], item.Nonce[:]) {
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
