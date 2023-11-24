package rate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestNoLimiter(t *testing.T) {
	l := &NoLimiter{}
	for i := 0; i < 10000; i++ {
		allowed, err := l.Allow("")
		assert.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestLocalRateLimiter(t *testing.T) {
	l := NewLocalRateLimiter(rate.Limit(2))

	for i := 0; i < 2; i++ {
		allowed, err := l.Allow("a")
		assert.NoError(t, err)
		assert.True(t, allowed)
	}

	allowed, err := l.Allow("a")
	assert.NoError(t, err)
	assert.False(t, allowed)

	// Ensure key partitioning is valid
	for i := 0; i < 2; i++ {
		allowed, err := l.Allow("b")
		assert.NoError(t, err)
		assert.True(t, allowed)
	}

	allowed, err = l.Allow("b")
	assert.NoError(t, err)
	assert.False(t, allowed)
}
