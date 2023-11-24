package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/config"
)

func TestHappyPath(t *testing.T) {
	c := NewConfig(nil)
	_, err := c.Get(context.Background())
	assert.Equal(t, config.ErrNoValue, err)

	expected := "value"
	c.SetValue(expected)
	val, err := c.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, val)

	c.ClearValue()
	_, err = c.Get(context.Background())
	assert.Equal(t, config.ErrNoValue, err)

	c.InduceErrors()
	_, err = c.Get(context.Background())
	assert.Equal(t, errDeveloperInduced, err)

	c.StopInducingErrors()
	_, err = c.Get(context.Background())
	assert.Equal(t, config.ErrNoValue, err)

	c.Shutdown()
	_, err = c.Get(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}
