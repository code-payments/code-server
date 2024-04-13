package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	mu      sync.Mutex
	records []*vault.Record
	last    uint64
	secret  string
}

type ById []*vault.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

func New() vault.Store {
	return &store{
		records: make([]*vault.Record, 0),
		last:    0,
		secret:  vault.GetSecret(),
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*vault.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

func (s *store) find(data *vault.Record) *vault.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.PublicKey == data.PublicKey {
			return item
		}
	}
	return nil
}

func (s *store) findPublicKey(pubkey string) *vault.Record {
	for _, item := range s.records {
		if item.PublicKey == pubkey {
			return item
		}
	}
	return nil
}

func (s *store) findByState(state vault.State) []*vault.Record {
	res := make([]*vault.Record, 0)
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) filter(items []*vault.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*vault.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*vault.Record
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

func (s *store) CountByState(ctx context.Context, state vault.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByState(state)
	return uint64(len(res)), nil
}

func (s *store) Save(ctx context.Context, data *vault.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.State = data.State
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		c := data.Clone()

		val, err := vault.Encrypt(c.PrivateKey, c.PublicKey)
		if err != nil {
			return err
		}
		c.PrivateKey = val

		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Get(ctx context.Context, sig string) (*vault.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findPublicKey(sig); item != nil {
		val, err := vault.Decrypt(item.PrivateKey, item.PublicKey)
		if err != nil {
			return nil, err
		}

		cloned := item.Clone()
		cloned.PrivateKey = val

		return &cloned, nil
	}

	return nil, vault.ErrKeyNotFound
}

func (s *store) GetAllByState(ctx context.Context, state vault.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*vault.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, vault.ErrKeyNotFound
		}

		return res, nil
	}

	return nil, vault.ErrKeyNotFound
}
