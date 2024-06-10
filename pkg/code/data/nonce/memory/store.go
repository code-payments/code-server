package memory

import (
	"context"
	"math/rand"
	"sort"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	mu      sync.Mutex
	records []*nonce.Record
	last    uint64
}

type ById []*nonce.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

func New() nonce.Store {
	return &store{
		records: make([]*nonce.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*nonce.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

func (s *store) find(data *nonce.Record) *nonce.Record {
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

func (s *store) findAddress(address string) *nonce.Record {
	for _, item := range s.records {
		if item.Address == address {
			return item
		}
	}
	return nil
}

func (s *store) findByState(state nonce.State) []*nonce.Record {
	return s.findFn(func(nonce *nonce.Record) bool {
		return nonce.State == state
	})
}

func (s *store) findByStateAndPurpose(state nonce.State, purpose nonce.Purpose) []*nonce.Record {
	return s.findFn(func(record *nonce.Record) bool {
		return record.State == state && record.Purpose == purpose
	})
}

func (s *store) findFn(f func(nonce *nonce.Record) bool) []*nonce.Record {
	res := make([]*nonce.Record, 0)
	for _, item := range s.records {
		if f(item) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filter(items []*nonce.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*nonce.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*nonce.Record
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

func (s *store) Count(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return uint64(len(s.records)), nil
}

func (s *store) CountByState(ctx context.Context, state nonce.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByState(state)
	return uint64(len(res)), nil
}

func (s *store) CountByStateAndPurpose(ctx context.Context, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByStateAndPurpose(state, purpose)
	return uint64(len(res)), nil
}

func (s *store) Save(ctx context.Context, data *nonce.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.Blockhash = data.Blockhash
		item.State = data.State
		item.Signature = data.Signature
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Get(ctx context.Context, address string) (*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findAddress(address); item != nil {
		return item, nil
	}

	return nil, nonce.ErrNonceNotFound
}

func (s *store) GetAllByState(ctx context.Context, state nonce.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, nonce.ErrNonceNotFound
		}

		return res, nil
	}

	return nil, nonce.ErrNonceNotFound
}

func (s *store) GetRandomAvailableByPurpose(ctx context.Context, purpose nonce.Purpose) (*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findFn(func(n *nonce.Record) bool {
		return n.Purpose == purpose && n.IsAvailable()
	})
	if len(items) == 0 {
		return nil, nonce.ErrNonceNotFound
	}

	index := rand.Intn(len(items))
	return items[index], nil
}
