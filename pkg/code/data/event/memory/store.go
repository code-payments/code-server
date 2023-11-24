package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/event"
)

type store struct {
	mu      sync.Mutex
	last    uint64
	records []*event.Record
}

// New returns a new in memory event.Store
func New() event.Store {
	return &store{}
}

// Save implements event.Store.Save
func (s *store) Save(_ context.Context, data *event.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.DestinationCodeAccount = pointer.StringCopy(data.DestinationCodeAccount)
		item.DestinationIdentity = pointer.StringCopy(data.DestinationIdentity)

		item.DestinationClientIp = pointer.StringCopy(data.DestinationClientIp)
		item.DestinationClientCity = pointer.StringCopy(data.DestinationClientCity)
		item.DestinationClientCountry = pointer.StringCopy(data.DestinationClientCountry)

		item.SpamConfidence = data.SpamConfidence

		item.CopyTo(data)
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}

		cloned := data.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// Get implements event.Store.Get
func (s *store) Get(_ context.Context, id string) (*event.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByEventId(id)
	if item == nil {
		return nil, event.ErrEventNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) find(data *event.Record) *event.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.EventId == data.EventId {
			return item
		}
	}

	return nil
}

func (s *store) findByEventId(id string) *event.Record {
	for _, item := range s.records {
		if item.EventId == id {
			return item
		}
	}

	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = 0
	s.records = nil
}
