package currency

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
)

type MultiRateRecord struct {
	Time  time.Time
	Rates map[string]float64
}

type ExchangeRateRecord struct {
	Id     uint64
	Time   time.Time
	Rate   float64
	Symbol string
}

var (
	ErrNotFound        = errors.New("price data not found")
	ErrInvalidRange    = errors.New("the provided range is not valid")
	ErrInvalidInterval = errors.New("the provided interval is not valid")
	ErrExists          = errors.New("price data for specified date exists")
)

type Store interface {
	// Put puts a record into the store.
	Put(ctx context.Context, record *MultiRateRecord) error

	// Get gets price information given a certain time and currency symbol. If
	// the exact time is not available, the most recent data prior to the
	// requested date within the same day will get returned, if available.
	//
	// ErrNotFound is returned if no price data was found for the provided Timestamp.
	Get(ctx context.Context, symbol string, t time.Time) (*ExchangeRateRecord, error)

	// GetAll gets price information given a certain time. If the exact time is
	// not available, the most recent data prior to the requested date within
	// the same day will get returned, if available.
	//
	// ErrNotFound is returned if no price data was found for the provided Timestamp.
	GetAll(ctx context.Context, t time.Time) (*MultiRateRecord, error)

	// GetRange gets the price information for a range of time given a currency
	// symbol and interval. The start and end timestamps are provided along with
	// the interval. If the raw data is not available at the sampling frequency
	// requested, it will be linearly interpolated between available points.
	//
	// ErrNotFound is returned if the symbol or the exchange rates for the symbol cannot be found
	// ErrInvalidRange is returned if the range is not valid
	// ErrInvalidInterval is returned if the interval is not valid
	GetRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*ExchangeRateRecord, error)
}
