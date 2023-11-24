package currency

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidBase = errors.New("invalid base currency")
)

type ExchangeData struct {
	Base      string
	Rates     map[string]float64
	Timestamp time.Time
}

type Client interface {
	// GetCurrentRates gets the current set of exchange rates against a base
	// currency.
	GetCurrentRates(ctx context.Context, base string) (*ExchangeData, error)

	// GetHistoricalRates gets the historical set of exchange rates against a
	// base currency. Granularity of time intervals may be significantly reduced.
	// A databse of cached rates from periodic calls to GetCurrentRates is the
	// recommended workaround.
	GetHistoricalRates(ctx context.Context, base string, timestamp time.Time) (*ExchangeData, error)
}
