package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
)

type store struct {
	mu      sync.Mutex
	last    uint64
	records []*storage.Record
}

// New returns a new in memory cvm.storage.Store
func New() storage.Store {
	return &store{}
}

// InitializeStorage implements cvm.storage.Store.InitializeStorage
func (s *store) InitializeStorage(_ context.Context, record *storage.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	if record.AvailableCapacity != storage.GetMaxCapacity(record.Levels) {
		return storage.ErrInvalidInitialCapacity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(record); item != nil {
		return storage.ErrAlreadyInitialized
	}

	record.Id = s.last
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	cloned := record.Clone()
	s.records = append(s.records, &cloned)

	return nil
}

// FindAnyWithAvailableCapacity implements cvm.storage.Store.FindAnyWithAvailableCapacity
func (s *store) FindAnyWithAvailableCapacity(_ context.Context, vm string, purpose storage.Purpose, minCapacity uint64) (*storage.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByVmAndPurpose(vm, purpose)
	items = s.filterByAvailableStorage(items, minCapacity)

	if len(items) == 0 {
		return nil, storage.ErrNotFound
	}

	cloned := items[0].Clone()
	return &cloned, nil
}

func (s *store) find(data *storage.Record) *storage.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.Address == data.Address {
			return item
		}
	}

	return nil
}

func (s *store) findByVmAndPurpose(vm string, purpose storage.Purpose) []*storage.Record {
	var res []*storage.Record
	for _, item := range s.records {
		if item.Vm != vm {
			continue
		}

		if item.Purpose != purpose {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) filterByAvailableStorage(items []*storage.Record, minCapacity uint64) []*storage.Record {
	var res []*storage.Record
	for _, item := range items {
		if item.AvailableCapacity >= minCapacity {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = 0
	s.records = nil
}
