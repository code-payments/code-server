package retry

import (
	"errors"
	"math"
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/code-payments/code-server/pkg/retry/backoff"
)

// Strategy is a function that determines whether or not an action should be
// retried. Strategies are allowed to delay or cause other side effects.
type Strategy func(attempts uint, err error) bool

// Limit returns a strategy that limits the total number of retries.
// maxAttempts should be >= 1, since the action is evaluateed first.
func Limit(maxAttempts uint) Strategy {
	return func(attempts uint, err error) bool {
		return attempts < maxAttempts
	}
}

// RetriableErrors returns a strategy that specifies which errors can be retried.
func RetriableErrors(retriableErrors ...error) Strategy {
	return func(attempts uint, err error) bool {
		for _, e := range retriableErrors {
			if errors.Is(err, e) {
				return true
			}
		}

		return false
	}
}

// NonRetriableErrors returns a strategy that specifies which errors should not be retried.
func NonRetriableErrors(nonRetriableErrors ...error) Strategy {
	return func(attempts uint, err error) bool {
		for _, e := range nonRetriableErrors {
			if errors.Is(err, e) {
				return false
			}
		}

		return true
	}
}

// Backoff returns a strategy that will delay the next retry, provided the
// action resulted in an error. The returned strategy will cause the caller
// (the retrier) to sleep.
func Backoff(strategy backoff.Strategy, maxBackoff time.Duration) Strategy {
	return func(attempts uint, err error) bool {
		delay := strategy(attempts)
		cappedDelay := time.Duration(math.Min(float64(maxBackoff), float64(delay)))
		sleeperImpl.Sleep(cappedDelay)
		return true
	}
}

// BackoffWithJitter returns a strategy similar to Backoff, but induces a jitter
// on the total delay. The maxBackoff is calculated before the jitter.
//
// The jitter parameter is a percentage of the total delay (after capping) that
// the timing can be off of. For example, a capped delay of 100ms with a jitter
// of 0.1 will result in a delay of 100ms +/- 10ms.
func BackoffWithJitter(strategy backoff.Strategy, maxBackoff time.Duration, jitter float64) Strategy {
	return func(attempts uint, err error) bool {
		delay := strategy(attempts)
		cappedDelay := time.Duration(math.Min(float64(maxBackoff), float64(delay)))

		// Center the jitter around the capped delay:
		//     <------cappedDelay------>
		//      jitter           jitter
		cappedDelayWithJitter := time.Duration(float64(cappedDelay) * (1 + (rand.Float64()*jitter*2 - jitter)))
		sleeperImpl.Sleep(cappedDelayWithJitter)
		return true
	}
}

// RetriableGRPCCodes returns a strategy that specifies which GRPC status codes can be retried.
func RetriableGRPCCodes(retriableCodes ...codes.Code) Strategy {
	return func(attempts uint, err error) bool {
		code := status.Code(err)
		for _, c := range retriableCodes {
			if code == c {
				return true
			}
		}

		return false
	}
}

// NonRetriableGRPCCodes returns a strategy that specifies which GRPC status codes should not be retried.
func NonRetriableGRPCCodes(nonRetriableCodes ...codes.Code) Strategy {
	return func(attempts uint, err error) bool {
		code := status.Code(err)
		for _, c := range nonRetriableCodes {
			if code == c {
				return false
			}
		}

		return true
	}
}

type sleeper interface {
	Sleep(time.Duration)
}

// realSleeper uses the time package to perform actual sleeps
type realSleeper struct{}

func (r *realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

var sleeperImpl sleeper = &realSleeper{}
