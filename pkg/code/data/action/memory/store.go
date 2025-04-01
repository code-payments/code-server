package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

type ById []*action.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type store struct {
	mu      sync.Mutex
	records []*action.Record
	last    uint64
}

func New() action.Store {
	return &store{
		records: make([]*action.Record, 0),
		last:    0,
	}
}

func (s *store) find(record *action.Record) *action.Record {
	for _, item := range s.records {
		if item.Id == record.Id {
			return item
		}

		if item.Intent == record.Intent && item.ActionId == record.ActionId {
			return item
		}
	}
	return nil
}

func (s *store) findById(intent string, actionId uint32) *action.Record {
	for _, item := range s.records {
		if item.Intent == intent && item.ActionId == actionId {
			return item
		}
	}
	return nil
}

func (s *store) findByIntent(intent string) []*action.Record {
	var res []*action.Record
	for _, item := range s.records {
		if item.Intent == intent {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByAddress(address string) []*action.Record {
	var res []*action.Record
	for _, item := range s.records {
		if item.Source == address {
			res = append(res, item)
			continue
		}

		if item.Destination != nil && *item.Destination == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findBySource(source string) []*action.Record {
	var res []*action.Record
	for _, item := range s.records {
		if item.Source == source {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) filter(items []*action.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*action.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*action.Record
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

func (s *store) filterByActionType(items []*action.Record, want action.Type) []*action.Record {
	var res []*action.Record
	for _, item := range items {
		if item.ActionType == want {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) filterByState(items []*action.Record, include bool, states ...action.State) []*action.Record {
	var res []*action.Record

	for _, item := range items {
		for _, state := range states {
			if item.State == state && include {
				res = append(res, item)
			} else if item.State != state && !include {
				res = append(res, item)
			}
		}
	}

	return res
}

// PutAll implements action.store.PutAll
func (s *store) PutAll(ctx context.Context, records ...*action.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	actionIds := make(map[string]struct{})
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}

		if record.Id > 0 {
			return action.ErrActionExists
		}

		key := fmt.Sprintf("%s:%d", record.Intent, record.ActionId)
		_, ok := actionIds[key]
		if ok {
			return action.ErrActionExists
		}
		actionIds[key] = struct{}{}

		if item := s.find(record); item != nil {
			return action.ErrActionExists
		}
	}

	for _, record := range records {
		s.last++

		record.Id = s.last
		if record.CreatedAt.IsZero() {
			record.CreatedAt = time.Now()
		}

		cloned := record.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// Update implements action.store.Update
func (s *store) Update(ctx context.Context, record *action.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.find(record); item != nil {
		if record.ActionType == action.CloseDormantAccount {
			item.Quantity = pointer.Uint64Copy(record.Quantity)
		}
		item.State = record.State
		return nil
	}

	return action.ErrActionNotFound
}

// GetById implements action.store.GetById
func (s *store) GetById(ctx context.Context, intent string, actionId uint32) (*action.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findById(intent, actionId)
	if item == nil {
		return nil, action.ErrActionNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetAllByIntent implements action.store.GetAllByIntent
func (s *store) GetAllByIntent(ctx context.Context, intent string) ([]*action.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByIntent(intent)
	if len(items) == 0 {
		return nil, action.ErrActionNotFound
	}

	copy := make([]*action.Record, len(items))
	for i, item := range items {
		cloned := item.Clone()
		copy[i] = &cloned
	}
	return copy, nil
}

// GetAllByAddress implements action.store.GetAllByAddress
func (s *store) GetAllByAddress(ctx context.Context, address string) ([]*action.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByAddress(address)
	if len(items) == 0 {
		return nil, action.ErrActionNotFound
	}

	copy := make([]*action.Record, len(items))
	for i, item := range items {
		cloned := item.Clone()
		copy[i] = &cloned
	}
	return copy, nil
}

// GetNetBalance implements action.store.GetNetBalance
func (s *store) GetNetBalance(ctx context.Context, account string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getNetBalance(account), nil
}

// GetNetBalanceBatch implements action.store.GetNetBalanceBatch
func (s *store) GetNetBalanceBatch(ctx context.Context, accounts ...string) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[string]int64)
	for _, account := range accounts {
		res[account] = s.getNetBalance(account)
	}
	return res, nil
}

// GetGiftCardClaimedAction implements action.store.GetGiftCardClaimedAction
func (s *store) GetGiftCardClaimedAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findBySource(giftCardVault)
	items = s.filterByActionType(items, action.NoPrivacyWithdraw)
	items = s.filterByState(items, false, action.StateRevoked)

	if len(items) == 0 {
		return nil, action.ErrActionNotFound
	} else if len(items) > 1 {
		return nil, action.ErrMultipleActionsFound
	}

	cloned := items[0].Clone()
	return &cloned, nil
}

// GetGiftCardAutoReturnAction implements action.store.GetGiftCardAutoReturnAction
func (s *store) GetGiftCardAutoReturnAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findBySource(giftCardVault)
	items = s.filterByActionType(items, action.CloseDormantAccount)
	items = s.filterByState(items, false, action.StateRevoked)

	if len(items) == 0 {
		return nil, action.ErrActionNotFound
	} else if len(items) > 1 {
		return nil, action.ErrMultipleActionsFound
	}

	cloned := items[0].Clone()
	return &cloned, nil
}

func (s *store) getNetBalance(account string) int64 {
	var res int64

	items := s.findByAddress(account)
	for _, item := range items {
		if item.State == action.StateRevoked {
			continue
		}

		if item.Quantity == nil {
			continue
		}

		if item.Source == account {
			res -= int64(*item.Quantity)
		}

		if item.Destination != nil && *item.Destination == account {
			res += int64(*item.Quantity)
		}
	}

	return res
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*action.Record, 0)
	s.last = 0
	s.mu.Unlock()
}
