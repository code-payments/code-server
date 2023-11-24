package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConstant(t *testing.T) {
	s := Constant(100 * time.Millisecond)

	for i := uint(1); i < 10; i++ {
		assert.Equal(t, 100*time.Millisecond, s(i))
	}
}

func TestLinear(t *testing.T) {
	s := Linear(500 * time.Millisecond)

	assert.Equal(t, 500*time.Millisecond, s(1))
	assert.Equal(t, 1*time.Second, s(2))
	assert.Equal(t, 1*time.Second+500*time.Millisecond, s(3))
	assert.Equal(t, 2*time.Second, s(4))
	assert.Equal(t, 2*time.Second+500*time.Millisecond, s(5))
}

func TestExponential(t *testing.T) {
	s := Exponential(2*time.Second, 3.0)

	assert.Equal(t, 2*time.Second, s(1))  // 2*3^0
	assert.Equal(t, 6*time.Second, s(2))  // 2*3^1
	assert.Equal(t, 18*time.Second, s(3)) // 2*3^2
	assert.Equal(t, 54*time.Second, s(4)) // 2*3^3
}

func TestBinaryExponential(t *testing.T) {
	exp := Exponential(2*time.Second, 2)
	binExp := BinaryExponential(2 * time.Second)

	for i := uint(1); i < 10; i++ {
		assert.Equal(t, exp(i), binExp(i))
	}
}
