package testutil

import (
	"time"

	"github.com/pkg/errors"
)

// WaitFor waits for a condition to be met before the specified timeout
func WaitFor(timeout, interval time.Duration, condition func() bool) error {
	if timeout < interval {
		return errors.New("timeout must be greater than interval")
	}
	start := time.Now()
	for {
		if condition() {
			return nil
		}
		if time.Since(start) >= timeout {
			return errors.Errorf("condition not met withind %v", timeout)
		}

		time.Sleep(interval)
	}
}
