package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
)

type ById []*treasury.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type store struct {
	mu                  sync.Mutex
	treasuryPoolRecords []*treasury.Record
	fundingRecords      []*treasury.FundingHistoryRecord
	last                uint64
}

// New returns a new in memory treasury.Store
func New() treasury.Store {
	return &store{}
}

// Save implements treasury.Store.Save
func (s *store) Save(_ context.Context, data *treasury.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.findTreasuryPool(data); item != nil {
		if data.SolanaBlock <= item.SolanaBlock {
			return treasury.ErrStaleTreasuryPoolState
		}

		historyList := make([]string, len(item.HistoryList))
		copy(historyList, data.HistoryList)

		item.SolanaBlock = data.SolanaBlock
		item.CurrentIndex = data.CurrentIndex
		item.HistoryList = historyList

		item.LastUpdatedAt = time.Now()

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		data.LastUpdatedAt = time.Now()
		c := data.Clone()
		s.treasuryPoolRecords = append(s.treasuryPoolRecords, c)
	}

	return nil
}

// GetByName implements treasury.Store.GetByName
func (s *store) GetByName(ctx context.Context, name string) (*treasury.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findTreasuryPoolByName(name); item != nil {
		return item.Clone(), nil
	}
	return nil, treasury.ErrTreasuryPoolNotFound
}

// GetByAddress implements treasury.Store.GetByAddress
func (s *store) GetByAddress(_ context.Context, address string) (*treasury.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findTreasuryPoolByAddress(address); item != nil {
		return item.Clone(), nil
	}
	return nil, treasury.ErrTreasuryPoolNotFound
}

// GetByVault implements treasury.Store.GetByVault
func (s *store) GetByVault(_ context.Context, vault string) (*treasury.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findTreasuryPoolByVault(vault); item != nil {
		return item.Clone(), nil
	}
	return nil, treasury.ErrTreasuryPoolNotFound
}

// GetAllByState implements treasury.Store.GetAllByState
func (s *store) GetAllByState(_ context.Context, state treasury.TreasuryPoolState, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*treasury.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findTreasuryPoolByState(state); len(items) > 0 {
		res := s.filterTreasuryPool(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, treasury.ErrTreasuryPoolNotFound
		}

		return res, nil
	}

	return nil, treasury.ErrTreasuryPoolNotFound
}

// SaveFunding implements treasury.Store.SaveFunding
func (s *store) SaveFunding(_ context.Context, data *treasury.FundingHistoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := data.Validate(); err != nil {
		return err
	}

	s.last++
	if item := s.findFunding(data); item != nil {
		item.State = data.State

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		c := data.Clone()
		s.fundingRecords = append(s.fundingRecords, c)
	}

	return nil
}

// GetTotalAvailableFunds implements treasury.Store.GetTotalAvailableFunds
func (s *store) GetTotalAvailableFunds(_ context.Context, vault string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res int64
	items := s.findFundingByVault(vault)
	for _, item := range items {
		if item.DeltaQuarks > 0 && item.State == treasury.FundingStateConfirmed {
			res += item.DeltaQuarks
		}

		if item.DeltaQuarks < 0 && item.State != treasury.FundingStateFailed {
			res += item.DeltaQuarks
		}
	}

	if res < 0 {
		return 0, treasury.ErrNegativeFunding
	}
	return uint64(res), nil
}

func (s *store) findTreasuryPool(data *treasury.Record) *treasury.Record {
	for _, item := range s.treasuryPoolRecords {
		if item.Id == data.Id {
			return item
		}
		if data.Address == item.Address {
			return item
		}
	}
	return nil
}

func (s *store) findTreasuryPoolByName(name string) *treasury.Record {
	for _, item := range s.treasuryPoolRecords {
		if name == item.Name {
			return item
		}
	}
	return nil
}

func (s *store) findTreasuryPoolByAddress(address string) *treasury.Record {
	for _, item := range s.treasuryPoolRecords {
		if address == item.Address {
			return item
		}
	}
	return nil
}

func (s *store) findTreasuryPoolByVault(vault string) *treasury.Record {
	for _, item := range s.treasuryPoolRecords {
		if vault == item.Vault {
			return item
		}
	}
	return nil
}

func (s *store) findTreasuryPoolByState(state treasury.TreasuryPoolState) []*treasury.Record {
	res := make([]*treasury.Record, 0)
	for _, item := range s.treasuryPoolRecords {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) filterTreasuryPool(items []*treasury.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*treasury.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*treasury.Record
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

func (s *store) findFunding(data *treasury.FundingHistoryRecord) *treasury.FundingHistoryRecord {
	for _, item := range s.fundingRecords {
		if item.Id == data.Id {
			return item
		}
		if data.TransactionId == item.TransactionId {
			return item
		}
	}
	return nil
}

func (s *store) findFundingByVault(vault string) []*treasury.FundingHistoryRecord {
	res := make([]*treasury.FundingHistoryRecord, 0)
	for _, item := range s.fundingRecords {
		if item.Vault == vault {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.treasuryPoolRecords = nil
	s.fundingRecords = nil
	s.last = 0
}
