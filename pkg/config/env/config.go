package env

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

type conf struct {
	val string
}

func NewConfig(key string) config.Config {
	client := &conf{
		val: os.Getenv(strings.ToUpper(key)),
	}

	return client
}

// Get implements Config.Get
func (c *conf) Get(ctx context.Context) (interface{}, error) {
	if len(c.val) == 0 {
		return nil, config.ErrNoValue
	}

	return []byte(c.val), nil
}

// Shutdown implements Config.Shutdown
func (c *conf) Shutdown() {
}

// NewBytesConfig creates a env-based byte array config
func NewBytesConfig(key string, defaultValue []byte) config.Bytes {
	return wrapper.NewBytesConfig(NewConfig(key), defaultValue)
}

// NewInt64Config creates a env-based int64 config
func NewInt64Config(key string, defaultValue int64) config.Int64 {
	return wrapper.NewInt64Config(NewConfig(key), defaultValue)
}

// NewUint64Config creates a env-based uint64 config
func NewUint64Config(key string, defaultValue uint64) config.Uint64 {
	return wrapper.NewUint64Config(NewConfig(key), defaultValue)
}

// NewFloat64Config creates a env-based float64 config
func NewFloat64Config(key string, defaultValue float64) config.Float64 {
	return wrapper.NewFloat64Config(NewConfig(key), defaultValue)
}

// NewStringConfig creates a env-based string config
func NewStringConfig(key string, defaultValue string) config.String {
	return wrapper.NewStringConfig(NewConfig(key), defaultValue)
}

// NewBoolConfig creates a env-based bool config
func NewBoolConfig(key string, defaultValue bool) config.Bool {
	return wrapper.NewBoolConfig(NewConfig(key), defaultValue)
}

// NewDurationConfig creates a env-based duration config
func NewDurationConfig(key string, defaultValue time.Duration) config.Duration {
	return wrapper.NewDurationConfig(NewConfig(key), defaultValue)
}
