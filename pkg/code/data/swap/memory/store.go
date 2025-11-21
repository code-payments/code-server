package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

type ById []*swap.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type store struct {
	mu      sync.RWMutex
	records []*swap.Record
	last    uint64
}

func New() swap.Store {
	return &store{}
}

func (s *store) Save(_ context.Context, data *swap.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		if item.Version != data.Version {
			return swap.ErrStaleVersion
		}

		data.Version++

		item.TransactionSignature = pointer.StringCopy(data.TransactionSignature)
		item.TransactionBlob = data.TransactionBlob
		item.State = data.State
		item.Version = data.Version
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		data.Version++

		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) GetById(_ context.Context, id string) (*swap.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item := s.findById(id)
	if item == nil {
		return nil, swap.ErrNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) GetAllByOwnerAndState(_ context.Context, owner string, state swap.State) ([]*swap.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.findByOwner(owner)
	items = s.filterByState(items, state)

	if len(items) == 0 {
		return nil, swap.ErrNotFound
	}
	return cloneRecords(items), nil
}

func (s *store) GetAllByState(_ context.Context, state swap.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*swap.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, swap.ErrNotFound
		}

		return res, nil
	}

	return nil, swap.ErrNotFound
}

func (s *store) find(data *swap.Record) *swap.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.SwapId == data.SwapId {
			return item
		}
	}
	return nil
}

func (s *store) findById(swapID string) *swap.Record {
	for _, item := range s.records {
		if item.SwapId == swapID {
			return item
		}
	}
	return nil
}

func (s *store) findByOwner(owner string) []*swap.Record {
	var res []*swap.Record
	for _, item := range s.records {
		if item.Owner == owner {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByState(state swap.State) []*swap.Record {
	var res []*swap.Record
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filter(items []*swap.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*swap.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*swap.Record
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

func (s *store) filterByState(items []*swap.Record, state swap.State) []*swap.Record {
	var res []*swap.Record
	for _, item := range items {
		if item.State == state {
			res = append(res, item)
		}
	}
	return res
}

func cloneRecords(items []*swap.Record) []*swap.Record {
	var res []*swap.Record
	for _, item := range items {
		cloned := item.Clone()
		res = append(res, &cloned)
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
