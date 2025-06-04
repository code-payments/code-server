package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

type store struct {
	mu      sync.Mutex
	last    uint64
	records []*deposit.Record
}

// New returns a new in memory deposit.Store
func New() deposit.Store {
	return &store{}
}

// Save implements deposit.Store.Save
func (s *store) Save(_ context.Context, data *deposit.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	if item := s.find(data); item != nil {
		item.Slot = data.Slot
		item.ConfirmationState = data.ConfirmationState

		item.CopyTo(data)

		return nil
	}

	data.Id = s.last
	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now()
	}

	cloned := data.Clone()
	s.records = append(s.records, &cloned)

	return nil
}

// Get implements deposit.Store.Get
func (s *store) Get(_ context.Context, signature, account string) (*deposit.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findBySignatureAndDestination(signature, account)
	if item == nil {
		return nil, deposit.ErrDepositNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetQuarkAmount implements deposit.Store.GetQuarkAmount
func (s *store) GetQuarkAmount(_ context.Context, account string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getQuarkAmount(account), nil
}

// GetQuarkAmountBatch implements deposit.Store.GetQuarkAmountBatch
func (s *store) GetQuarkAmountBatch(_ context.Context, accounts ...string) (map[string]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[string]uint64)
	for _, account := range accounts {
		res[account] = s.getQuarkAmount(account)
	}
	return res, nil
}

// GetUsdAmount implements deposit.Store.GetUsdAmount
func (s *store) GetUsdAmount(ctx context.Context, account string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getUsdAmount(account), nil
}

func (s *store) getQuarkAmount(account string) uint64 {
	items := s.findByDestination(account)
	items = s.filterFinalized(items)
	return s.sumAmounts(items)
}

func (s *store) getUsdAmount(account string) float64 {
	items := s.findByDestination(account)
	items = s.filterFinalized(items)
	return s.sumUsd(items)
}

func (s *store) find(data *deposit.Record) *deposit.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.Signature == data.Signature && item.Destination == data.Destination {
			return item
		}
	}
	return nil
}

func (s *store) findByDestination(account string) []*deposit.Record {
	var res []*deposit.Record
	for _, item := range s.records {
		if item.Destination == account {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findBySignatureAndDestination(signature, destination string) *deposit.Record {
	for _, item := range s.records {
		if item.Signature == signature && item.Destination == destination {
			return item
		}
	}
	return nil
}

func (s *store) filterFinalized(items []*deposit.Record) []*deposit.Record {
	var res []*deposit.Record
	for _, item := range items {
		if item.ConfirmationState == transaction.ConfirmationFinalized {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) sumAmounts(items []*deposit.Record) uint64 {
	var res uint64
	for _, item := range items {
		res += item.Amount
	}
	return res
}

func (s *store) sumUsd(items []*deposit.Record) float64 {
	var res float64
	for _, item := range items {
		res += item.UsdMarketValue
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = 0
	s.records = nil
}
