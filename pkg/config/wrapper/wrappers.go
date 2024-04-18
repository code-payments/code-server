package wrapper

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/config"
)

// ErrUnsuportedConversion indicates the wrapper does not implement conversion from the source type
var ErrUnsuportedConversion = errors.New("config: wrapper conversion from source type not implemented")

// BytesConfig is a utility wrapper for a byte array config
type BytesConfig struct {
	override     config.Config
	defaultValue []byte

	stateMu   sync.RWMutex
	lastValue []byte
}

// NewBytesConfig returns a new byte array config utility wrapper
func NewBytesConfig(override config.Config, defaultValue []byte) config.Bytes {
	return &BytesConfig{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *BytesConfig) GetSafe(ctx context.Context) ([]byte, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *BytesConfig) Get(ctx context.Context) []byte {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *BytesConfig) Shutdown() {
	c.override.Shutdown()
}

// BoolConfig is a utility wrapper for a bool config
type BoolConfig struct {
	override     config.Config
	defaultValue bool

	stateMu   sync.RWMutex
	lastValue bool
}

// NewBoolConfig returns a new bool config utility wrapper
func NewBoolConfig(override config.Config, defaultValue bool) config.Bool {
	return &BoolConfig{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *BoolConfig) GetSafe(ctx context.Context) (bool, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue, err := strconv.ParseBool(string(override))
		if err != nil {
			return lastValue, err
		}
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case bool:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *BoolConfig) Get(ctx context.Context) bool {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *BoolConfig) Shutdown() {
	c.override.Shutdown()
}

// Int64Config is a utility wrapper for a int64 config
type Int64Config struct {
	override     config.Config
	defaultValue int64

	stateMu   sync.RWMutex
	lastValue int64
}

// NewInt64Config returns a new int64 config utility wrapper
func NewInt64Config(override config.Config, defaultValue int64) config.Int64 {
	return &Int64Config{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *Int64Config) GetSafe(ctx context.Context) (int64, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue, err := strconv.ParseInt(string(override), 10, 64)
		if err != nil {
			return lastValue, err
		}
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case int64:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case int:
		newValue := int64(override)
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *Int64Config) Get(ctx context.Context) int64 {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *Int64Config) Shutdown() {
	c.override.Shutdown()
}

// Uint64Config is a utility wrapper for a uint64 config
type Uint64Config struct {
	override     config.Config
	defaultValue uint64

	stateMu   sync.RWMutex
	lastValue uint64
}

// NewUint64Config returns a new uint64 config utility wrapper
func NewUint64Config(override config.Config, defaultValue uint64) config.Uint64 {
	return &Uint64Config{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *Uint64Config) GetSafe(ctx context.Context) (uint64, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue, err := strconv.ParseUint(string(override), 10, 64)
		if err != nil {
			return lastValue, err
		}
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case uint64:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case uint:
		newValue := uint64(override)
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *Uint64Config) Get(ctx context.Context) uint64 {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *Uint64Config) Shutdown() {
	c.override.Shutdown()
}

// Float64Config is a utility wrapper for a float64 config
type Float64Config struct {
	override     config.Config
	defaultValue float64

	stateMu   sync.RWMutex
	lastValue float64
}

// NewFloat64Config returns a new float64 config utility wrapper
func NewFloat64Config(override config.Config, defaultValue float64) config.Float64 {
	return &Float64Config{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *Float64Config) GetSafe(ctx context.Context) (float64, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue, err := strconv.ParseFloat(string(override), 64)
		if err != nil {
			return lastValue, err
		}
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case float64:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *Float64Config) Get(ctx context.Context) float64 {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *Float64Config) Shutdown() {
	c.override.Shutdown()
}

// StringConfig is a utility wrapper for a string config
type StringConfig struct {
	config       config.Config
	defaultValue string

	stateMu   sync.RWMutex
	lastValue string
}

// NewStringConfig returns a new duration string utility wrapper
func NewStringConfig(config config.Config, defaultValue string) config.String {
	return &StringConfig{
		config:       config,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *StringConfig) GetSafe(ctx context.Context) (string, error) {
	override, err := c.config.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		newValue := string(override)
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case string:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *StringConfig) Get(ctx context.Context) string {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *StringConfig) Shutdown() {
	c.config.Shutdown()
}

// DurationConfig is a utility wrapper for a duration config
type DurationConfig struct {
	override     config.Config
	defaultValue time.Duration

	stateMu   sync.RWMutex
	lastValue time.Duration
}

// NewDurationConfig returns a new duration config utility wrapper
func NewDurationConfig(override config.Config, defaultValue time.Duration) config.Duration {
	return &DurationConfig{
		override:     override,
		defaultValue: defaultValue,
		lastValue:    defaultValue,
	}
}

// GetSafe gets a config value and propagates any errors that arise. A best-effort
// attempt is made to return the last known value
func (c *DurationConfig) GetSafe(ctx context.Context) (time.Duration, error) {
	override, err := c.override.Get(ctx)
	c.stateMu.RLock()
	lastValue := c.lastValue
	c.stateMu.RUnlock()
	if err == config.ErrNoValue {
		c.stateMu.Lock()
		c.lastValue = c.defaultValue
		c.stateMu.Unlock()
		return c.defaultValue, nil
	} else if err != nil {
		return lastValue, err
	}
	switch override := override.(type) {
	case []byte:
		var newValue time.Duration
		strValue := string(override)

		newValue, err = time.ParseDuration(strValue)
		if err != nil {
			return lastValue, err
		}
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	case time.Duration:
		newValue := override
		c.stateMu.Lock()
		c.lastValue = newValue
		c.stateMu.Unlock()
		return newValue, nil
	default:
		return lastValue, ErrUnsuportedConversion
	}
}

// Get is a wrapper for GetSafe that ignores the returned error
func (c *DurationConfig) Get(ctx context.Context) time.Duration {
	val, _ := c.GetSafe(ctx)
	return val
}

// Shutdown signals the config to stop all underlying resources
func (c *DurationConfig) Shutdown() {
	c.override.Shutdown()
}
