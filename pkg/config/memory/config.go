package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/code-payments/code-server/pkg/config"
)

var errDeveloperInduced = errors.New("in memory config: developer induced error")

// Config is an in memory config used for testing
type Config struct {
	stateMu  sync.RWMutex
	value    interface{}
	err      error
	shutdown bool
}

// NewConfig returns a new in memory config. Use an initial nil value to indicate
// no value is set
func NewConfig(value interface{}) *Config {
	return &Config{
		value: value,
		err:   nil,
	}
}

// Get implements Config.Get
func (c *Config) Get(_ context.Context) (interface{}, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	if c.shutdown {
		return nil, config.ErrShutdown
	}

	if c.err != nil {
		return nil, c.err
	}
	if c.value == nil {
		return nil, config.ErrNoValue
	}
	return c.value, nil
}

// Shutdown implements Config.Shutdown
func (c *Config) Shutdown() {
	c.stateMu.Lock()
	c.shutdown = true
	c.stateMu.Unlock()
}

// SetValue sets the value that should be returned on subsequent Get calls
func (c *Config) SetValue(value interface{}) {
	c.stateMu.Lock()
	c.value = value
	c.stateMu.Unlock()
}

// ClearValue sets up the config as if no value has been set, resulting in
// ErrNoValue being returned on subsequent Get Calls
func (c *Config) ClearValue() {
	c.stateMu.Lock()
	c.value = nil
	c.stateMu.Unlock()
}

// InduceErrors instructs the config to simulate an error getting a config value
func (c *Config) InduceErrors() {
	c.stateMu.Lock()
	c.err = errDeveloperInduced
	c.stateMu.Unlock()
}

// StopInducingErrors stops the config from simulating an error getting a config value
func (c *Config) StopInducingErrors() {
	c.stateMu.Lock()
	c.err = nil
	c.stateMu.Unlock()
}
