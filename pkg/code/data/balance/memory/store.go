package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/balance"
)

type store struct {
	mu      sync.Mutex
	records []*balance.Record
	last    uint64
}

// New returns a new in memory balance.Store
func New() balance.Store {
	return &store{}
}

// SaveCheckpoint implements balance.Store.SaveCheckpoint
func (s *store) SaveCheckpoint(_ context.Context, data *balance.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		if data.SlotCheckpoint <= item.SlotCheckpoint {
			return balance.ErrStaleCheckpoint
		}

		item.SlotCheckpoint = data.SlotCheckpoint
		item.Quarks = data.Quarks
		item.LastUpdatedAt = time.Now()
		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		data.LastUpdatedAt = time.Now()
		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

// GetCheckpoint implements balance.Store.GetCheckpoint
func (s *store) GetCheckpoint(_ context.Context, account string) (*balance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByTokenAccount(account); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}
	return nil, balance.ErrCheckpointNotFound
}

func (s *store) find(data *balance.Record) *balance.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if data.TokenAccount == item.TokenAccount {
			return item
		}
	}
	return nil
}

func (s *store) findByTokenAccount(account string) *balance.Record {
	for _, item := range s.records {
		if account == item.TokenAccount {
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
