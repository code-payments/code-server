package retry

// Action is a function to be performed in a retriable manner.
type Action func() error

// Retrier retries the provided action.
type Retrier interface {
	Retry(action Action) (uint, error)
}

type retrier struct {
	strategies []Strategy
}

// NewRetrier returns a Retrier that will retry actions based off of the
// provided strategies. If no strategies are provided, the retrier acts
// as a tight-loop, retrying until no error is returned from the action.
func NewRetrier(strategies ...Strategy) Retrier {
	return &retrier{
		strategies: strategies,
	}
}

func (r *retrier) Retry(action Action) (uint, error) {
	return Retry(action, r.strategies...)
}

// Retry executes the provided action, potentially multiple times based off of
// the provided strategies. Retry will block until the action is successful, or
// one of the provided strategies indicate no further retries should be performed.
//
// The strategies are executed in the provided order, so any strategies that
// induce delays should be specified last.
func Retry(action Action, strategies ...Strategy) (uint, error) {
	for i := uint(1); ; i++ {
		err := action()
		if err == nil {
			return i, nil
		}

		for _, s := range strategies {
			if shouldRetry := s(i, err); !shouldRetry {
				return i, err
			}
		}
	}
}

// Loop executes the provided action infinitely, until one of the provided
// strategies indicates it should not be retried.
//
// Unlike Retry, when the action returns with no error, the internal attempt counter
// is reset, and the action is retried.
func Loop(action Action, strategies ...Strategy) error {
	for i := uint(1); ; i++ {
		err := action()
		if err == nil {
			i = 0
			continue
		}

		for _, s := range strategies {
			if shouldRetry := s(i, err); !shouldRetry {
				return err
			}
		}
	}
}
