package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	mu      sync.Mutex
	records []*intent.Record
	last    uint64
}

type ById []*intent.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type ByCreatedAt []*intent.Record

func (a ByCreatedAt) Len() int           { return len(a) }
func (a ByCreatedAt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByCreatedAt) Less(i, j int) bool { return a[i].CreatedAt.Before(a[j].CreatedAt) }

func New() intent.Store {
	return &store{
		records: make([]*intent.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*intent.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

func (s *store) find(data *intent.Record) *intent.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.IntentId == data.IntentId {
			return item
		}
	}
	return nil
}

func (s *store) findIntent(intentID string) *intent.Record {
	for _, item := range s.records {
		if item.IntentId == intentID {
			return item
		}
	}
	return nil
}

func (s *store) findByState(state intent.State) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByOwner(owner string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.InitiatorOwnerAccount == owner {
			res = append(res, item)
			continue
		}

		if item.SendPublicPaymentMetadata != nil && item.SendPublicPaymentMetadata.DestinationOwnerAccount == owner {
			res = append(res, item)
			continue
		}

		if item.ExternalDepositMetadata != nil && item.ExternalDepositMetadata.DestinationOwnerAccount == owner {
			res = append(res, item)
			continue
		}
	}

	return res
}

func (s *store) findByDestination(destination string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		switch item.IntentType {
		case intent.ExternalDeposit:
			if item.ExternalDepositMetadata.DestinationTokenAccount == destination {
				res = append(res, item)
			}
		case intent.SendPublicPayment:
			if item.SendPublicPaymentMetadata.DestinationTokenAccount == destination {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findBySource(source string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		switch item.IntentType {
		case intent.ReceivePaymentsPublicly:
			if item.ReceivePaymentsPubliclyMetadata.Source == source {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findByInitiatorAndType(intentType intent.Type, owner string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.IntentType != intentType {
			continue
		}

		if item.InitiatorOwnerAccount != owner {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) filter(items []*intent.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*intent.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*intent.Record
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

func (s *store) filterByState(items []*intent.Record, include bool, states ...intent.State) []*intent.Record {
	var res []*intent.Record

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

func (s *store) filterByType(items []*intent.Record, intentType intent.Type) []*intent.Record {
	var res []*intent.Record

	for _, item := range items {
		if item.IntentType == intentType {
			res = append(res, item)
		}
	}

	return res
}

func (s *store) filterByRemoteSendFlag(items []*intent.Record, want bool) []*intent.Record {
	var res []*intent.Record
	for _, item := range items {
		switch item.IntentType {
		case intent.ReceivePaymentsPublicly:
			if item.ReceivePaymentsPubliclyMetadata.IsRemoteSend == want {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) Save(ctx context.Context, data *intent.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		// Only update state
		item.State = data.State
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Get(ctx context.Context, intentID string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findIntent(intentID); item != nil {
		return item, nil
	}

	return nil, intent.ErrIntentNotFound
}

func (s *store) GetAllByOwner(ctx context.Context, owner string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByOwner(owner); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, intent.ErrIntentNotFound
		}

		return res, nil
	}

	return nil, intent.ErrIntentNotFound
}

func (s *store) GetLatestByInitiatorAndType(ctx context.Context, intentType intent.Type, owner string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByInitiatorAndType(intentType, owner)
	if len(items) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	latest := items[0]
	for _, item := range items {
		if item.CreatedAt.After(latest.CreatedAt) {
			latest = item
		}
	}

	return latest, nil
}

func (s *store) GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	return nil, errors.New("not implemented")

	/*
		s.mu.Lock()
		defer s.mu.Unlock()

		items := s.findByDestination(giftCardVault)
		items = s.filterByType(items, intent.SendPrivatePayment)
		items = s.filterByState(items, false, intent.StateRevoked)
		items = s.filterByRemoteSendFlag(items, true)

		if len(items) == 0 {
			return nil, intent.ErrIntentNotFound
		}

		if len(items) > 1 {
			return nil, intent.ErrMultilpeIntentsFound
		}

		cloned := items[0].Clone()
		return &cloned, nil
	*/
}

func (s *store) GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	return nil, errors.New("not implemented")

	/*
		s.mu.Lock()
		defer s.mu.Unlock()

		items := s.findBySource(giftCardVault)
		items = s.filterByType(items, intent.ReceivePaymentsPublicly)
		items = s.filterByState(items, false, intent.StateRevoked)
		items = s.filterByRemoteSendFlag(items, true)

		if len(items) == 0 {
			return nil, intent.ErrIntentNotFound
		}

		if len(items) > 1 {
			return nil, intent.ErrMultilpeIntentsFound
		}

		cloned := items[0].Clone()
		return &cloned, nil
	*/
}
