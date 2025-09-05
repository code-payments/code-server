package currency

import (
	"time"
)

const (
	exchangeRateUpdatesPerHour = 4
	timePerExchangeRateUpdate  = time.Hour / exchangeRateUpdatesPerHour
)

// GetLatestExchangeRateTime gets the latest time for fetching an exchange rate.
// By synchronizing on a time, we can eliminate the amount of perceived volatility
// over short time spans.
func GetLatestExchangeRateTime() time.Time {
	// Standardize to concrete 15 minute intervals to reduce perceived volatility.
	// Notably, don't fall exactly on the 15 minute interval, so we remove 1 second.
	// The way our postgres DB query is setup, the start of UTC day is unlikely to
	// generate results.
	secondsInUpdateInterval := int64(timePerExchangeRateUpdate / time.Second)
	queryTimeUnix := time.Now().Unix()
	queryTimeUnix = queryTimeUnix - (queryTimeUnix % secondsInUpdateInterval) - 1
	return time.Unix(queryTimeUnix, 0)
}
