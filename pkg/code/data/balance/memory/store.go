package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/balance"
)

type store struct {
	mu                             sync.Mutex
	cachedBalanceVersionsByAccount map[string]uint64
	externalCheckpointRecords      []*balance.ExternalCheckpointRecord
	last                           uint64
}

// New returns a new in memory balance.Store
func New() balance.Store {
	return &store{
		cachedBalanceVersionsByAccount: make(map[string]uint64),
	}
}

// GetCachedVersion implements balance.Store.GetCachedVersion
func (s *store) GetCachedVersion(_ context.Context, account string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.cachedBalanceVersionsByAccount[account]
	if !ok {
		return 0, nil
	}
	return current, nil
}

// AdvanceCachedVersion implements balance.Store.AdvanceCachedVersion
func (s *store) AdvanceCachedVersion(_ context.Context, account string, currentVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	actualVersion, ok := s.cachedBalanceVersionsByAccount[account]
	if !ok {
		if currentVersion != 0 {
			return balance.ErrStaleCachedBalanceVersion
		}

		s.cachedBalanceVersionsByAccount[account] = 1

		return nil
	}

	if actualVersion != currentVersion {
		return balance.ErrStaleCachedBalanceVersion
	}

	s.cachedBalanceVersionsByAccount[account]++

	return nil
}

// SaveExternalCheckpoint implements balance.Store.SaveExternalCheckpoint
func (s *store) SaveExternalCheckpoint(_ context.Context, data *balance.ExternalCheckpointRecord) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.findExternalCheckpoint(data); item != nil {
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
		s.externalCheckpointRecords = append(s.externalCheckpointRecords, &c)
	}

	return nil
}

// GetExternalCheckpoint implements balance.Store.GetExternalCheckpoint
func (s *store) GetExternalCheckpoint(_ context.Context, account string) (*balance.ExternalCheckpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findExternalCheckpointByTokenAccount(account); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}
	return nil, balance.ErrCheckpointNotFound
}

func (s *store) findExternalCheckpoint(data *balance.ExternalCheckpointRecord) *balance.ExternalCheckpointRecord {
	for _, item := range s.externalCheckpointRecords {
		if item.Id == data.Id {
			return item
		}
		if data.TokenAccount == item.TokenAccount {
			return item
		}
	}
	return nil
}

func (s *store) findExternalCheckpointByTokenAccount(account string) *balance.ExternalCheckpointRecord {
	for _, item := range s.externalCheckpointRecords {
		if account == item.TokenAccount {
			return item
		}
	}
	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cachedBalanceVersionsByAccount = make(map[string]uint64)
	s.externalCheckpointRecords = nil
	s.last = 0
}
