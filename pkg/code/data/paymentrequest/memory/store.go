package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
)

type store struct {
	mu      sync.Mutex
	records []*paymentrequest.Record
	last    uint64
}

func New() paymentrequest.Store {
	return &store{
		records: make([]*paymentrequest.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*paymentrequest.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

// Put implements paymentrequest.Store.Put
func (s *store) Put(_ context.Context, data *paymentrequest.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		return paymentrequest.ErrPaymentRequestAlreadyExists
	} else {
		seenDestinations := make(map[string]any)
		for _, fee := range data.Fees {
			_, ok := seenDestinations[fee.DestinationTokenAccount]
			if ok {
				return paymentrequest.ErrInvalidPaymentRequest
			}
			seenDestinations[fee.DestinationTokenAccount] = true
		}

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

// Get implements paymentrequest.Store.Get
func (s *store) Get(_ context.Context, intentId string) (*paymentrequest.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByIntent(intentId)
	if item == nil {
		return nil, paymentrequest.ErrPaymentRequestNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) find(data *paymentrequest.Record) *paymentrequest.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.Intent == data.Intent {
			return item
		}
	}
	return nil
}

func (s *store) findByIntent(intentId string) *paymentrequest.Record {
	for _, item := range s.records {
		if item.Intent == intentId {
			return item
		}
	}
	return nil
}
