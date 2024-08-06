package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type store struct {
	mu                     sync.Mutex
	last                   uint64
	records                []*ram.Record
	reservedAccountIndices map[string]struct{}
}

// New returns a new in memory vm.ram.Store
func New() ram.Store {
	return &store{
		reservedAccountIndices: make(map[string]struct{}),
	}
}

// InitializeMemory implements vm.ram.Store.InitializeMemory
func (s *store) InitializeMemory(_ context.Context, record *ram.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(record); item != nil {
		return ram.ErrAlreadyInitialized
	}

	record.Id = s.last
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	cloned := record.Clone()
	s.records = append(s.records, &cloned)

	return nil
}

// FreeMemory implements vm.ram.Store.FreeMemory
func (s *store) FreeMemory(_ context.Context, memoryAccount string, index uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := getAccountIndexKey(memoryAccount, index)
	if _, ok := s.reservedAccountIndices[key]; !ok {
		return ram.ErrNotReserved
	}

	delete(s.reservedAccountIndices, key)

	return nil
}

// ReserveMemory implements vm.ram.Store.ReserveMemory
func (s *store) ReserveMemory(_ context.Context, vm string, accountType cvm.VirtualAccountType) (string, uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByVmAndAccountType(vm, accountType)
	for _, item := range items {
		actualCapacity := ram.GetActualCapcity(item)
		for i := 0; i < int(actualCapacity); i++ {
			key := getAccountIndexKey(item.Address, uint16(i))

			if _, ok := s.reservedAccountIndices[key]; ok {
				continue
			}

			s.reservedAccountIndices[key] = struct{}{}
			return item.Address, uint16(i), nil
		}
	}

	return "", 0, ram.ErrNoFreeMemory
}

func (s *store) find(data *ram.Record) *ram.Record {
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

func (s *store) findByVmAndAccountType(vm string, accountType cvm.VirtualAccountType) []*ram.Record {
	var res []*ram.Record
	for _, item := range s.records {
		if item.Vm != vm {
			continue
		}

		if item.StoredAccountType != accountType {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = 0
	s.records = nil
	s.reservedAccountIndices = make(map[string]struct{})
}

func getAccountIndexKey(memoryAccount string, index uint16) string {
	return fmt.Sprintf("%s:%d", memoryAccount, index)
}
