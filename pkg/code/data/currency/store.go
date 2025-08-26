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

type ReserveRecord struct {
	Id                uint64
	Mint              string
	SupplyFromBonding uint64
	CoreMintLocked    uint64
	Time              time.Time
}

var (
	ErrNotFound        = errors.New("record not found")
	ErrInvalidRange    = errors.New("the provided range is not valid")
	ErrInvalidInterval = errors.New("the provided interval is not valid")
	ErrExists          = errors.New("record for specified date exists")
)

type Store interface {
	// PutExchangeRates puts exchange rate records into the store.
	PutExchangeRates(ctx context.Context, record *MultiRateRecord) error

	// GetExchangeRate gets price information given a certain time and currency symbol.
	// If the exact time is not available, the most recent data prior to the
	// requested date within the same day will get returned, if available.
	//
	// ErrNotFound is returned if no price data was found for the provided Timestamp.
	GetExchangeRate(ctx context.Context, symbol string, t time.Time) (*ExchangeRateRecord, error)

	// GetAll gets price information given a certain time. If the exact time is
	// not available, the most recent data prior to the requested date within
	// the same day will get returned, if available.
	//
	// ErrNotFound is returned if no price data was found for the provided Timestamp.
	GetAllExchangeRates(ctx context.Context, t time.Time) (*MultiRateRecord, error)

	// GetExchangeRatesInRange gets the price information for a range of time
	// given a currency symbol and interval. The start and end timestamps are provided
	// along with the interval. If the raw data is not available at the sampling frequency
	// requested, it will be linearly interpolated between available points.
	//
	// ErrNotFound is returned if the symbol or the exchange rates for the symbol cannot be found
	// ErrInvalidRange is returned if the range is not valid
	// ErrInvalidInterval is returned if the interval is not valid
	GetExchangeRatesInRange(ctx context.Context, symbol string, interval query.Interval, start time.Time, end time.Time, ordering query.Ordering) ([]*ExchangeRateRecord, error)

	// PutReserveRecord puts a mint reserve records into the store.
	PutReserveRecord(ctx context.Context, record *ReserveRecord) error

	// GetReserveAtTime gets reserve state for a given mint at a point in time.
	// If the exact time is not available, the most recent data prior to the
	// requested date within the same day will get returned, if available.
	//
	// ErrNotFound is returned if no reserve data was found for the provided Timestamp.
	GetReserveAtTime(ctx context.Context, mint string, t time.Time) (*ReserveRecord, error)
}
