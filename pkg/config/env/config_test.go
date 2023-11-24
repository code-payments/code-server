package env

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/code-payments/code-server/pkg/config"
)

func TestConfigDoesntExist(t *testing.T) {
	const env = "ENV_CONFIG_TEST_VAR"
	os.Setenv(env, "default")

	v, err := NewConfig(env).Get(context.Background())
	assert.Equal(t, []byte("default"), v)
	assert.Nil(t, err)

	os.Unsetenv(env)

	v, err = NewConfig(env).Get(context.Background())
	assert.Nil(t, v)
	assert.Equal(t, config.ErrNoValue, err)
}
