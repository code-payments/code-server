package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/database/query"
)

type ById []*commitment.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type store struct {
	mu      sync.Mutex
	records []*commitment.Record
	last    uint64
}

// New returns a new in memory commitment.Store
func New() commitment.Store {
	return &store{}
}

// Save implements commitment.Store.Save
func (s *store) Save(_ context.Context, data *commitment.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		if item.TreasuryRepaid && !data.TreasuryRepaid {
			return commitment.ErrInvalidCommitment
		}

		if item.RepaymentDivertedTo != nil && data.RepaymentDivertedTo == nil {
			return commitment.ErrInvalidCommitment
		}

		if item.RepaymentDivertedTo != nil && data.RepaymentDivertedTo != nil && *item.RepaymentDivertedTo != *data.RepaymentDivertedTo {
			return commitment.ErrInvalidCommitment
		}

		if item.State > data.State {
			return commitment.ErrInvalidCommitment
		}

		item.TreasuryRepaid = data.TreasuryRepaid
		item.RepaymentDivertedTo = data.RepaymentDivertedTo
		item.State = data.State

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		s.records = append(s.records, data.Clone())
	}

	return nil
}

// GetByAddress implements commitment.Store.GetByAddress
func (s *store) GetByAddress(_ context.Context, address string) (*commitment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByAddress(address); item != nil {
		return item.Clone(), nil
	}

	return nil, commitment.ErrCommitmentNotFound
}

// GetByAction implements commitment.Store.GetByAction
func (s *store) GetByAction(_ context.Context, intentId string, actionId uint32) (*commitment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByAction(intentId, actionId); item != nil {
		return item.Clone(), nil
	}

	return nil, commitment.ErrCommitmentNotFound
}

// GetAllByState implements commitment.Store.GetAllByState
func (s *store) GetAllByState(ctx context.Context, state commitment.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*commitment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, commitment.ErrCommitmentNotFound
		}

		return res, nil
	}

	return nil, commitment.ErrCommitmentNotFound
}

// GetUpgradeableByOwner implements commitment.Store.GetUpgradeableByOwner
func (s *store) GetUpgradeableByOwner(_ context.Context, owner string, limit uint64) ([]*commitment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByOwner(owner)
	items = s.filterByNilRepaymentDivertedTo(items)
	items = s.filterByStates(
		items,
		false,
		commitment.StateUnknown,
		commitment.StatePayingDestination,
		commitment.StateReadyToRemoveFromMerkleTree,
		commitment.StateRemovedFromMerkleTree,
	)

	if len(items) > int(limit) {
		items = items[:limit]
	}

	if len(items) == 0 {
		return nil, commitment.ErrCommitmentNotFound
	}
	return items, nil
}

// GetUsedTreasuryPoolDeficit implements commitment.Store.GetUsedTreasuryPoolDeficit
func (s *store) GetUsedTreasuryPoolDeficit(_ context.Context, pool string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByPool(pool)
	items = s.filterByStates(items, false, commitment.StateUnknown)
	items = s.filterByRepaymentStatus(items, false)
	return s.sumQuantity(items), nil
}

// GetTotalTreasuryPoolDeficit implements commitment.Store.GetTotalTreasuryPoolDeficit
func (s *store) GetTotalTreasuryPoolDeficit(_ context.Context, pool string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByPool(pool)
	items = s.filterByRepaymentStatus(items, false)
	return s.sumQuantity(items), nil
}

// CountByState implements commitment.Store.CountByState
func (s *store) CountByState(_ context.Context, state commitment.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(state)
	return uint64(len(items)), nil
}

// CountPendingRepaymentsDivertedToCommitment implements commitment.Store.CountPendingRepaymentsDivertedToCommitment
func (s *store) CountPendingRepaymentsDivertedToCommitment(_ context.Context, address string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByRepaymentsDivertedToCommitment(address)
	items = s.filterByRepaymentStatus(items, false)
	return uint64(len(items)), nil
}

func (s *store) find(data *commitment.Record) *commitment.Record {
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

func (s *store) findByAddress(address string) *commitment.Record {
	for _, item := range s.records {
		if item.Address == address {
			return item
		}
	}
	return nil
}

func (s *store) findByAction(intentId string, actionId uint32) *commitment.Record {
	for _, item := range s.records {
		if item.Intent == intentId && item.ActionId == actionId {
			return item
		}
	}
	return nil
}

func (s *store) findByPool(pool string) []*commitment.Record {
	var res []*commitment.Record
	for _, item := range s.records {
		if item.Pool == pool {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByRepaymentsDivertedToCommitment(address string) []*commitment.Record {
	var res []*commitment.Record
	for _, item := range s.records {
		if item.RepaymentDivertedTo != nil && *item.RepaymentDivertedTo == address {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByState(state commitment.State) []*commitment.Record {
	res := make([]*commitment.Record, 0)
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByOwner(owner string) []*commitment.Record {
	res := make([]*commitment.Record, 0)
	for _, item := range s.records {
		if item.Owner == owner {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterByStates(items []*commitment.Record, include bool, states ...commitment.State) []*commitment.Record {
	var res []*commitment.Record
	for _, item := range items {
		var inState bool
		for _, state := range states {
			if item.State == state {
				inState = true
				break
			}
		}

		if (include && inState) || (!include && !inState) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterByRepaymentStatus(items []*commitment.Record, want bool) []*commitment.Record {
	var res []*commitment.Record
	for _, item := range items {
		if item.TreasuryRepaid == want {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterByNilRepaymentDivertedTo(items []*commitment.Record) []*commitment.Record {
	var res []*commitment.Record
	for _, item := range items {
		if item.RepaymentDivertedTo == nil {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filter(items []*commitment.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*commitment.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*commitment.Record
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

func (s *store) sumQuantity(items []*commitment.Record) uint64 {
	var res uint64
	for _, item := range items {
		res += item.Amount
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
