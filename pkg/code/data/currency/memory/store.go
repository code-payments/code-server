package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/database/query"
)

const (
	dateFormat = "2006-01-02"
)

type store struct {
	mu                    sync.Mutex
	exchangeRateRecords   []*currency.ExchangeRateRecord
	lastExchangeRateIndex uint64
	metadataRecords       []*currency.MetadataRecord
	lastMetadataIndex     uint64
	reserveRecords        []*currency.ReserveRecord
	lastReserveIndex      uint64
}

type RateByTime []*currency.ExchangeRateRecord

func (a RateByTime) Len() int      { return len(a) }
func (a RateByTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a RateByTime) Less(i, j int) bool {
	// DESC order (most recent first)
	return a[i].Time.Unix() > a[j].Time.Unix()
}

type ReserveByTime []*currency.ReserveRecord

func (a ReserveByTime) Len() int      { return len(a) }
func (a ReserveByTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ReserveByTime) Less(i, j int) bool {
	// DESC order (most recent first)
	return a[i].Time.Unix() > a[j].Time.Unix()
}

func New() currency.Store {
	return &store{
		exchangeRateRecords:   make([]*currency.ExchangeRateRecord, 0),
		lastExchangeRateIndex: 1,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.exchangeRateRecords = make([]*currency.ExchangeRateRecord, 0)
	s.lastExchangeRateIndex = 1
	s.metadataRecords = make([]*currency.MetadataRecord, 0)
	s.lastMetadataIndex = 1
	s.reserveRecords = make([]*currency.ReserveRecord, 0)
	s.lastReserveIndex = 1
	s.mu.Unlock()
}

func (s *store) PutExchangeRates(ctx context.Context, data *currency.MultiRateRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	for _, item := range s.exchangeRateRecords {
		if item.Time.Unix() == data.Time.Unix() {
			return currency.ErrExists
		}
	}

	for symbol, item := range data.Rates {
		s.exchangeRateRecords = append(s.exchangeRateRecords, &currency.ExchangeRateRecord{
			Id:     s.lastExchangeRateIndex,
			Rate:   item,
			Time:   data.Time,
			Symbol: symbol,
		})
		s.lastExchangeRateIndex = s.lastExchangeRateIndex + 1
	}

	return nil
}

func (s *store) GetExchangeRate(ctx context.Context, symbol string, t time.Time) (*currency.ExchangeRateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	var results []*currency.ExchangeRateRecord
	for _, item := range s.exchangeRateRecords {
		if item.Symbol == symbol && item.Time.Unix() <= t.Unix() && item.Time.Format(dateFormat) == t.Format(dateFormat) {
			results = append(results, item)
		}
	}

	if len(results) == 0 {
		return nil, currency.ErrNotFound
	}

	sort.Sort(RateByTime(results))

	return results[0], nil
}

func (s *store) GetAllExchangeRates(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	sort.Sort(RateByTime(s.exchangeRateRecords))

	result := currency.MultiRateRecord{
		Rates: make(map[string]float64),
	}
	for _, item := range s.exchangeRateRecords {
		if item.Time.Unix() <= t.Unix() && item.Time.Format(dateFormat) == t.Format(dateFormat) {
			result.Rates[item.Symbol] = item.Rate
			result.Time = item.Time
		}
	}

	if len(result.Rates) == 0 {
		return nil, currency.ErrNotFound
	}

	return &result, nil
}

func (s *store) GetExchangeRatesInRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*currency.ExchangeRateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sort.Sort(RateByTime(s.exchangeRateRecords))

	// Not ideal but fine for testing the currency store
	var all []*currency.ExchangeRateRecord
	for _, item := range s.exchangeRateRecords {
		if item.Symbol == symbol && item.Time.Unix() >= start.Unix() && item.Time.Unix() <= end.Unix() {
			all = append(all, item)
		}
	}

	// TODO: handle the interval

	if len(all) == 0 {
		return nil, currency.ErrNotFound
	}

	if ordering == query.Ascending {
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
	}

	return all, nil
}

func (s *store) PutMetadata(ctx context.Context, data *currency.MetadataRecord) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	for _, item := range s.metadataRecords {
		if item.Mint == data.Mint {
			return currency.ErrExists
		}
	}

	data.Id = s.lastMetadataIndex
	s.metadataRecords = append(s.metadataRecords, data.Clone())
	s.lastMetadataIndex = s.lastMetadataIndex + 1

	return nil
}

func (s *store) GetMetadata(ctx context.Context, mint string) (*currency.MetadataRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.metadataRecords {
		if item.Mint == mint {
			return item.Clone(), nil
		}
	}

	return nil, currency.ErrNotFound
}

func (s *store) PutReserveRecord(ctx context.Context, data *currency.ReserveRecord) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	for _, item := range s.reserveRecords {
		if item.Mint == data.Mint && item.Time.Unix() == data.Time.Unix() {
			return currency.ErrExists
		}
	}

	data.Id = s.lastReserveIndex
	s.reserveRecords = append(s.reserveRecords, data.Clone())
	s.lastReserveIndex = s.lastReserveIndex + 1

	return nil
}

func (s *store) GetReserveAtTime(ctx context.Context, mint string, t time.Time) (*currency.ReserveRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Not ideal but fine for testing the currency store
	var results []*currency.ReserveRecord
	for _, item := range s.reserveRecords {
		if item.Mint == mint && item.Time.Unix() <= t.Unix() && item.Time.Format(dateFormat) == t.Format(dateFormat) {
			results = append(results, item)
		}
	}

	if len(results) == 0 {
		return nil, currency.ErrNotFound
	}

	sort.Sort(ReserveByTime(results))

	return results[0].Clone(), nil
}
