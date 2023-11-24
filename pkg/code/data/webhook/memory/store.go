package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

type store struct {
	mu      sync.Mutex
	last    uint64
	records []*webhook.Record
}

// New returns a new in memory webhook.Store
func New() webhook.Store {
	return &store{}
}

// Put implements webhook.Store.Put
func (s *store) Put(_ context.Context, data *webhook.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		return webhook.ErrAlreadyExists
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		data.CreatedAt = time.Now()

		cloned := data.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// Update implements webhook.Store.Update
func (s *store) Update(_ context.Context, data *webhook.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.Attempts = data.Attempts
		item.State = data.State
		item.NextAttemptAt = pointer.TimeCopy(data.NextAttemptAt)

		item.CopyTo(data)
	} else {
		return webhook.ErrNotFound
	}

	return nil
}

// Get implements webhook.Store.Get
func (s *store) Get(_ context.Context, webhookId string) (*webhook.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByWebhookId(webhookId)
	if item == nil {
		return nil, webhook.ErrNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// CountByState implements webhook.Store.CountByState
func (s *store) CountByState(_ context.Context, state webhook.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(state)
	return uint64(len(items)), nil
}

// GetAllPendingReadyToSend implements webhook.Store.GetAllPendingReadyToSend
func (s *store) GetAllPendingReadyToSend(_ context.Context, limit uint64) ([]*webhook.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(webhook.StatePending)
	items = s.filterNextAttemptBeforeOrAt(items, time.Now())

	if len(items) == 0 {
		return nil, webhook.ErrNotFound
	} else if uint64(len(items)) > limit {
		items = items[:limit]
	}
	return cloneSlice(items), nil
}

func (s *store) find(data *webhook.Record) *webhook.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.WebhookId == data.WebhookId {
			return item
		}
	}

	return nil
}

func (s *store) findByWebhookId(webhookId string) *webhook.Record {
	for _, item := range s.records {
		if item.WebhookId == webhookId {
			return item
		}
	}

	return nil
}

func (s *store) findByState(state webhook.State) []*webhook.Record {
	var res []*webhook.Record

	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
		}
	}

	return res
}

func (s *store) filterNextAttemptBeforeOrAt(items []*webhook.Record, ts time.Time) []*webhook.Record {
	var res []*webhook.Record

	for _, item := range items {
		if item.NextAttemptAt != nil && item.NextAttemptAt.Compare(ts) <= 0 {
			res = append(res, item)
		}
	}

	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = 0
	s.records = nil
}

func cloneSlice(items []*webhook.Record) []*webhook.Record {
	var res []*webhook.Record
	for _, item := range items {
		cloned := item.Clone()
		res = append(res, &cloned)
	}
	return res
}
