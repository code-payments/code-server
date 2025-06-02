package currency

import (
	"time"
)

// todo: add tests, but generally well tested in server tests since that's where most of this originated
// todo: does this belong in an exchange-specific package?

// GetLatestExchangeRateTime gets the latest time for fetching an exchange rate.
// By synchronizing on a time, we can eliminate the amount of perceived volatility
// over short time spans.
func GetLatestExchangeRateTime() time.Time {
	// Standardize to concrete 15 minute intervals to reduce perceived volatility.
	// Notably, don't fall exactly on the 15 minute interval, so we remove 1 second.
	// The way our postgres DB query is setup, the start of UTC day is unlikely to
	// generate results.
	secondsIn15Minutes := int64(15 * time.Minute / time.Second)
	queryTimeUnix := time.Now().Unix()
	queryTimeUnix = queryTimeUnix - (queryTimeUnix % secondsIn15Minutes) - 1
	return time.Unix(queryTimeUnix, 0)
}
