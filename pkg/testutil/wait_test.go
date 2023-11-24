package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitFor(t *testing.T) {
	require.NoError(t, WaitFor(50*time.Millisecond, 25*time.Millisecond, func() bool {
		return true
	}))

	require.Error(t, WaitFor(50*time.Millisecond, 25*time.Millisecond, func() bool {
		return false
	}))

	require.Error(t, WaitFor(50*time.Millisecond, 100*time.Millisecond, func() bool {
		return true
	}))
}
