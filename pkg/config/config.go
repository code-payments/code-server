package config

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

var (
	// ErrNoValue indicates no value was set for the config
	ErrNoValue = errors.New("config: no value set")

	// ErrShutdown indicates the use of a Config after calling Shutdown
	ErrShutdown = errors.New("config: shutdown")
)

// Config is an interface for getting a configuration value
type Config interface {
	// Get returns the latest config value
	Get(ctx context.Context) (interface{}, error)

	// Shutdown signals the config to stop all underlying resources
	Shutdown()
}

// NoopConfig is a config that does not yield any values.
var NoopConfig = &noopConfig{}

type noopConfig struct{}

func (*noopConfig) Get(_ context.Context) (interface{}, error) {
	return nil, ErrNoValue
}

func (*noopConfig) Shutdown() {
}

// Bool provides a boolean typed config.Config.
type Bool interface {
	Get(ctx context.Context) bool
	GetSafe(ctx context.Context) (bool, error)
	Shutdown()
}

// Bytes provides a bytes typed config.Config.
type Bytes interface {
	Get(ctx context.Context) []byte
	GetSafe(ctx context.Context) ([]byte, error)
	Shutdown()
}

// Duration provides a time.Duration typed config.Config.
type Duration interface {
	Get(ctx context.Context) time.Duration
	GetSafe(ctx context.Context) (time.Duration, error)
	Shutdown()
}

// Encrypted provides an encrypted bytes typed config.Config.
type Encrypted interface {
	Get(ctx context.Context) []byte
	GetSafe(ctx context.Context) ([]byte, error)
	Shutdown()
}

// Float64 provides a float64 typed config.Config.
type Float64 interface {
	Get(ctx context.Context) float64
	GetSafe(ctx context.Context) (float64, error)
	Shutdown()
}

// Int64 provides an int64 typed config.Config.
type Int64 interface {
	Get(ctx context.Context) int64
	GetSafe(ctx context.Context) (int64, error)
	Shutdown()
}

// Uint64 provides a uint64 typed config.Config.
type Uint64 interface {
	Get(ctx context.Context) uint64
	GetSafe(ctx context.Context) (uint64, error)
	Shutdown()
}

// String provides a string typed config.Config.
type String interface {
	Get(ctx context.Context) string
	GetSafe(ctx context.Context) (string, error)
	Shutdown()
}
