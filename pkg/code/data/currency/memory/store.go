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
	currencyStoreMu sync.Mutex
	currencyStore   []*currency.ExchangeRateRecord
	lastIndex       uint64
}

type ByTime []*currency.ExchangeRateRecord

func (a ByTime) Len() int      { return len(a) }
func (a ByTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTime) Less(i, j int) bool {
	// DESC order (most recent first)
	return a[i].Time.Unix() > a[j].Time.Unix()
}

func New() currency.Store {
	return &store{
		currencyStore: make([]*currency.ExchangeRateRecord, 0),
		lastIndex:     1,
	}
}

func (s *store) reset() { //nolint:unused
	s.currencyStoreMu.Lock()
	s.currencyStore = make([]*currency.ExchangeRateRecord, 0)
	s.currencyStoreMu.Unlock()
}

func (s *store) Put(ctx context.Context, data *currency.MultiRateRecord) error {
	s.currencyStoreMu.Lock()
	defer s.currencyStoreMu.Unlock()

	// Not ideal but fine for testing the currency store
	for _, item := range s.currencyStore {
		if item.Time.Unix() == data.Time.Unix() {
			return currency.ErrExists
		}
	}

	for symbol, item := range data.Rates {
		s.currencyStore = append(s.currencyStore, &currency.ExchangeRateRecord{
			Id:     s.lastIndex,
			Rate:   item,
			Time:   data.Time,
			Symbol: symbol,
		})
		s.lastIndex = s.lastIndex + 1
	}

	return nil
}

func (s *store) Get(ctx context.Context, symbol string, t time.Time) (*currency.ExchangeRateRecord, error) {
	s.currencyStoreMu.Lock()
	defer s.currencyStoreMu.Unlock()

	// Not ideal but fine for testing the currency store
	var results []*currency.ExchangeRateRecord
	for _, item := range s.currencyStore {
		if item.Symbol == symbol && item.Time.Unix() <= t.Unix() {
			results = append(results, item)
		}
	}

	if len(results) == 0 {
		return nil, currency.ErrNotFound
	}

	sort.Sort(ByTime(results))

	return results[0], nil
}

func (s *store) GetAll(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	s.currencyStoreMu.Lock()
	defer s.currencyStoreMu.Unlock()

	// Not ideal but fine for testing the currency store
	sort.Sort(ByTime(s.currencyStore))

	result := currency.MultiRateRecord{
		Rates: make(map[string]float64),
	}
	for _, item := range s.currencyStore {
		if item.Time.Unix() <= t.Unix() && item.Time.Format(dateFormat) == t.Format(dateFormat) {
			result.Rates[item.Symbol] = item.Rate
		}
	}

	if len(result.Rates) == 0 {
		return nil, currency.ErrNotFound
	}

	return &result, nil
}

func (s *store) GetRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*currency.ExchangeRateRecord, error) {
	s.currencyStoreMu.Lock()
	defer s.currencyStoreMu.Unlock()

	sort.Sort(ByTime(s.currencyStore))

	// Not ideal but fine for testing the currency store
	var all []*currency.ExchangeRateRecord
	for _, item := range s.currencyStore {
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
