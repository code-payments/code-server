package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

type store struct {
	mu      sync.Mutex
	records []*timelock.Record
	last    uint64
}

type ById []*timelock.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

// New returns a new in memory timelock.Store
func New() timelock.Store {
	return &store{}
}

// Save implements timelock.Store.Save
func (s *store) Save(_ context.Context, data *timelock.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		if data.Block <= item.Block {
			return timelock.ErrStaleTimelockState
		}

		var unlockAt *uint64
		if data.UnlockAt != nil {
			value := *data.UnlockAt
			unlockAt = &value
		}

		item.VaultState = data.VaultState
		item.UnlockAt = unlockAt

		item.Block = data.Block

		item.LastUpdatedAt = time.Now()

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		data.LastUpdatedAt = time.Now()
		c := data.Clone()
		s.records = append(s.records, c)
	}

	return nil
}

// GetByAddress implements timelock.Store.GetByAddress
func (s *store) GetByAddress(_ context.Context, address string) (*timelock.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByAddress(address); item != nil {
		return item.Clone(), nil
	}
	return nil, timelock.ErrTimelockNotFound
}

// GetByVault implements timelock.Store.GetByVault
func (s *store) GetByVault(_ context.Context, vault string) (*timelock.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByVault(vault); item != nil {
		return item.Clone(), nil
	}
	return nil, timelock.ErrTimelockNotFound
}

func (s *store) GetByDepositPda(ctx context.Context, depositPda string) (*timelock.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByDepositPda(depositPda); item != nil {
		return item.Clone(), nil
	}
	return nil, timelock.ErrTimelockNotFound
}

// GetByVaultBatch implements timelock.Store.GetByVaultBatch
func (s *store) GetByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[string]*timelock.Record)
	for _, vault := range vaults {
		item := s.findByVault(vault)
		if item == nil {
			return nil, timelock.ErrTimelockNotFound
		}

		res[vault] = item.Clone()
	}
	return res, nil
}

// GetAllByState implements timelock.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state timelock_token.TimelockState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*timelock.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, timelock.ErrTimelockNotFound
		}

		return res, nil
	}

	return nil, timelock.ErrTimelockNotFound
}

// GetCountByState implements timelock.Store.GetCountByState
func (s *store) GetCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(state)
	return uint64(len(items)), nil
}

func (s *store) find(data *timelock.Record) *timelock.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if data.Address == item.Address {
			return item
		}
	}
	return nil
}

func (s *store) findByAddress(address string) *timelock.Record {
	for _, item := range s.records {
		if address == item.Address {
			return item
		}
	}
	return nil
}

func (s *store) findByVault(vault string) *timelock.Record {
	for _, item := range s.records {
		if vault == item.VaultAddress {
			return item
		}
	}
	return nil
}

func (s *store) findByDepositPda(depositPda string) *timelock.Record {
	for _, item := range s.records {
		if depositPda == item.DepositPdaAddress {
			return item
		}
	}
	return nil
}

func (s *store) findByState(state timelock_token.TimelockState) []*timelock.Record {
	res := make([]*timelock.Record, 0)
	for _, item := range s.records {
		if item.VaultState == state {
			res = append(res, item.Clone())
		}
	}
	return res
}

func (s *store) filter(items []*timelock.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*timelock.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*timelock.Record
	for _, item := range items {
		if item.Id > start && direction == query.Ascending {
			res = append(res, item)
		}
		if item.Id < start && direction == query.Descending {
			res = append(res, item)
		}
	}

	if direction == query.Descending {
		sort.Sort(sort.Reverse(ById(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit]
	}

	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
