package retry

import (
	"math"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/code-payments/code-server/pkg/retry/backoff"
)

func TestLimit(t *testing.T) {
	strategy := Limit(2)

	// One iteration has been executed. Try again.
	assert.True(t, strategy(1, errors.New("test")))
	// Two iterations have been executred. Do not try again.
	assert.False(t, strategy(2, errors.New("test")))

	counter, err := Retry(func() error {
		return errors.New("test")
	}, Limit(2))

	assert.EqualError(t, err, "test")
	assert.Equal(t, uint(2), counter)
}

func TestRetriableErrors(t *testing.T) {
	retriableErrors := []error{
		errors.New("retriableA"),
		errors.New("retriableB"),
		errors.New("retriableC"),
	}

	strategy := RetriableErrors(retriableErrors...)
	for _, err := range retriableErrors {
		assert.True(t, strategy(1, err))
		// Ensure wrapped errors are detected.
		assert.True(t, strategy(1, errors.Wrap(err, "wrapper")))
	}
	assert.False(t, strategy(2, errors.New("unexpected")))
}

func TestNonRetriableErrors(t *testing.T) {
	nonRetriableErrors := []error{
		errors.New("nonRetriableA"),
		errors.New("nonRetriableB"),
		errors.New("nonRetriableC"),
	}

	strategy := NonRetriableErrors(nonRetriableErrors...)
	for _, err := range nonRetriableErrors {
		assert.False(t, strategy(1, err))
		// Ensure wrapped errors are detected.
		assert.False(t, strategy(1, errors.Wrap(err, "wrapper")))
	}
	assert.True(t, strategy(1, errors.New("unexpected")))
}

func TestBackoff(t *testing.T) {
	sleeperImpl = &testSleeper{}
	strategy := Backoff(backoff.Constant(100*time.Millisecond), 1*time.Second)

	for i := uint(0); i < 10; i++ {
		assert.True(t, strategy(i+1, errors.New("test-error")))
	}

	assert.EqualValues(t, 1*time.Second, sleeperImpl.(*testSleeper).Total())
	assert.EqualValues(t, 100*time.Millisecond, sleeperImpl.(*testSleeper).Mean())
	assert.EqualValues(t, 0*time.Second, sleeperImpl.(*testSleeper).AbsDeviation())
}

func TestBackoffWithJitter(t *testing.T) {
	iterations := 10000
	delay := 1 * time.Millisecond

	sleeperImpl = &testSleeper{}
	strategy := BackoffWithJitter(backoff.Constant(delay), delay, 0.1)

	for i := 0; i < iterations; i++ {
		assert.True(t, strategy(1, errors.New("err")))
	}

	// We expect that the total time slept is (iterations * delay) +/- 10%
	assert.InDelta(t,
		float64(10*time.Second),
		float64(sleeperImpl.(*testSleeper).Total()),
		float64(1*time.Second),
	)

	// We expect the mean to be the delay +/- 10%
	assert.InDelta(t,
		float64(delay),
		float64(sleeperImpl.(*testSleeper).Mean()),
		0.1*float64(delay),
	)

	// We expect the deviation to be 5% of the delay, since we provided
	// a 10% jitter. This is because there is a 10% window of jitter around
	// the mean, which results in a deviation of 5%.
	assert.InDelta(t,
		0.05*float64(delay),
		float64(sleeperImpl.(*testSleeper).AbsDeviation()),
		0.05*0.05*float64(delay), // accurate within 5%
	)
}

func TestRetriableGRPCCodes(t *testing.T) {
	retriableCodes := []codes.Code{
		codes.Internal,
		codes.ResourceExhausted,
		codes.Unavailable,
	}

	strategy := RetriableGRPCCodes(retriableCodes...)
	for _, c := range retriableCodes {
		assert.True(t, strategy(1, status.Error(c, "msg")))
	}
	assert.False(t, strategy(2, status.Error(codes.FailedPrecondition, "msg")))
}

func TestNonRetriableGRPCCodes(t *testing.T) {
	nonRetriableCodes := []codes.Code{
		codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
	}

	strategy := NonRetriableGRPCCodes(nonRetriableCodes...)
	for _, c := range nonRetriableCodes {
		assert.False(t, strategy(1, status.Error(c, "msg")))
	}
	assert.True(t, strategy(1, status.Error(codes.Internal, "msg")))
}

type testSleeper struct {
	// We keep a history of sleepTimes, since it makes it significantly
	// easier to compute metrics about the sleeps. The memory complexity
	// is acceptable since it's well bounded within these tests.
	sleepTimes []time.Duration
}

func (t *testSleeper) Sleep(d time.Duration) {
	t.sleepTimes = append(t.sleepTimes, d)
}

func (t *testSleeper) Total() (total time.Duration) {
	for _, d := range t.sleepTimes {
		total += d
	}
	return total
}

func (t *testSleeper) Mean() (mean time.Duration) {
	for _, d := range t.sleepTimes {
		mean += d
	}
	return time.Duration(int(mean) / len(t.sleepTimes))
}

func (t *testSleeper) AbsDeviation() (dev time.Duration) {
	mean := t.Mean()
	for _, d := range t.sleepTimes {
		dev += time.Duration(math.Abs((float64(d) - float64(mean))))
	}
	return time.Duration(int(dev) / len(t.sleepTimes))
}
