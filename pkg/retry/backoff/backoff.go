// Package backoff provides varies backoff strategies for retry.
package backoff

import (
	"math"
	"time"
)

// Strategy is a function that provides the amount of time to wait before trying
// again. Note: attempts starts at 1
type Strategy func(attempts uint) time.Duration

// Constant returns a strategy that always returns the provided duration.
func Constant(interval time.Duration) Strategy {
	return func(attempts uint) time.Duration {
		return interval
	}
}

// Linear returns a strategy that linearly increases based off of the number of
// attempts.
//
// delay = baseDelay * attempts
// Ex. Linear(2*time.Seconds) = 2s, 4s, 6s, 8s, ...
func Linear(baseDelay time.Duration) Strategy {
	return func(attempts uint) time.Duration {
		if delay := baseDelay * time.Duration(attempts); delay >= 0 {
			return delay
		}

		return math.MaxInt64
	}
}

// Exponential returns a strategy that exponentially increases based off of the
// number of attempts.
//
// delay = baseDelay * base^(attempts - 1)
// Ex. Exponential(2*time.Seconds, 3) = 2s, 6s, 18s, 54s, ...
func Exponential(baseDelay time.Duration, base float64) Strategy {
	return func(attempts uint) time.Duration {
		if delay := baseDelay * time.Duration(math.Pow(base, float64(attempts-1))); delay >= 0 {
			return delay
		}

		return math.MaxInt64
	}
}

// BinaryExponential returns an Exponential strategy with a base of 2.0
//
// delay = baseDelay * 2^(attempts - 1)
// Ex. BinaryExponential(2*time.Seconds) = 2s, 4s, 8s, 16s, ...
func BinaryExponential(baseDelay time.Duration) Strategy {
	return Exponential(baseDelay, 2)
}
