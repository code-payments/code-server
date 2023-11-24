package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/code-payments/code-server/pkg/retry/backoff"
)

func TestRealSleeper(t *testing.T) {
	sleeperImpl = &realSleeper{}

	start := time.Now()
	n, err := Retry(func() error { return errors.New("err") },
		Limit(2),
		Backoff(backoff.Constant(500*time.Millisecond), 500*time.Millisecond),
	)

	assert.NotNil(t, err)
	assert.EqualValues(t, 2, n)
	assert.True(t, 500*time.Millisecond <= time.Since(start))
	assert.True(t, 1*time.Second > time.Since(start))
}

func TestRetrier(t *testing.T) {
	retriableErr := errors.New("retriable")
	r := NewRetrier(Limit(5), RetriableErrors(retriableErr))

	// Happy path always goes through
	attempts, err := r.Retry(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, uint(1), attempts)

	// Test ordering does not matter, by triggering 1 filter, then the other.
	attempts, err = r.Retry(func() error { return errors.New("unknown") })
	assert.Error(t, err)
	assert.Equal(t, uint(1), attempts)

	attempts, err = r.Retry(func() error { return retriableErr })
	assert.EqualError(t, retriableErr, err.Error())
	assert.Equal(t, uint(5), attempts)
}

func TestLoop(t *testing.T) {
	ts := &testSleeper{}
	sleeperImpl = ts

	var errNonRetriable = errors.New("non retriable")

	var i int
	err := Loop(
		func() error {
			defer func() { i++ }()

			if i > 10 {
				return errNonRetriable
			}

			if i%4 == 0 {
				return nil
			}

			return errors.New("blah")
		},
		NonRetriableErrors(errNonRetriable),
		Backoff(backoff.Linear(1), time.Second),
	)
	assert.Error(t, err, errNonRetriable)
	assert.Equal(t, []time.Duration{1, 2, 3, 1, 2, 3, 1, 2}, ts.sleepTimes)
}
