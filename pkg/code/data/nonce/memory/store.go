package memory

import (
	"context"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
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

func (s *store) findByState(env nonce.Environment, instance string, state nonce.State) []*nonce.Record {
	res := make([]*nonce.Record, 0)
	for _, item := range s.findByEnvironmentInstance(env, instance) {
		if item.State == state {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByStateAndPurpose(env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) []*nonce.Record {
	res := make([]*nonce.Record, 0)
	for _, item := range s.findByEnvironmentInstance(env, instance) {
		if item.State != state {
			continue
		}

		if item.Purpose != purpose {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) findByEnvironmentInstance(env nonce.Environment, instance string) []*nonce.Record {
	res := make([]*nonce.Record, 0)
	for _, item := range s.records {
		if item.Environment != env {
			continue
		}

		if item.EnvironmentInstance != instance {
			continue
		}

		res = append(res, item)
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

func (s *store) filterAvailableToClaim(items []*nonce.Record) []*nonce.Record {
	var res []*nonce.Record
	for _, item := range items {
		if item.IsAvailableToClaim() {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) Count(ctx context.Context, env nonce.Environment, instance string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByEnvironmentInstance(env, instance)
	return uint64(len(res)), nil
}

func (s *store) CountByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByState(env, instance, state)
	return uint64(len(res)), nil
}

func (s *store) CountByStateAndPurpose(ctx context.Context, env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByStateAndPurpose(env, instance, state, purpose)
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
		if item.Version != data.Version {
			return nonce.ErrStaleVersion
		}

		data.Version++

		item.Blockhash = data.Blockhash
		item.Signature = data.Signature
		item.State = data.State
		item.ClaimNodeID = pointer.StringCopy(data.ClaimNodeID)
		item.ClaimExpiresAt = pointer.TimeCopy(data.ClaimExpiresAt)
		item.Version = data.Version
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}

		data.Version++

		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Get(ctx context.Context, address string) (*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findAddress(address); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}

	return nil, nonce.ErrNonceNotFound
}

func (s *store) GetAllByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(env, instance, state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, nonce.ErrNonceNotFound
		}

		return clonedRecords(res), nil
	}

	return nil, nonce.ErrNonceNotFound
}

func (s *store) BatchClaimAvailableByPurpose(ctx context.Context, env nonce.Environment, instance string, purpose nonce.Purpose, limit int, nodeID string, minExpireAt, maxExpireAt time.Time) ([]*nonce.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByStateAndPurpose(env, instance, nonce.StateAvailable, purpose)
	items = append(items, s.findByStateAndPurpose(env, instance, nonce.StateClaimed, purpose)...)
	items = s.filterAvailableToClaim(items)
	if len(items) == 0 {
		return nil, nonce.ErrNonceNotFound
	}
	if len(items) > limit {
		items = items[:limit]
	}

	for i, l := 0, len(items); i < l; i++ {
		j := rand.Intn(l)
		items[i], items[j] = items[j], items[i]
	}
	for i := 0; i < len(items); i++ {
		window := maxExpireAt.Sub(minExpireAt)
		expiry := minExpireAt.Add(time.Duration(rand.Intn(int(window))))

		items[i].State = nonce.StateClaimed
		items[i].ClaimNodeID = pointer.String(nodeID)
		items[i].ClaimExpiresAt = pointer.Time(expiry)
		items[i].Version++
	}

	return clonedRecords(items), nil
}

func clonedRecords(items []*nonce.Record) []*nonce.Record {
	res := make([]*nonce.Record, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}
