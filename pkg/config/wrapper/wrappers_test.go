package wrapper

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/memory"
)

func TestBytesConfig(t *testing.T) {
	defaultValue := []byte("default")
	overridenValue := []byte("override")
	mock := memory.NewConfig(nil)
	wrapper := NewBytesConfig(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))
}

func TestBoolConfig(t *testing.T) {
	defaultValue := true
	overridenValue := false
	mock := memory.NewConfig(nil)
	wrapper := NewBoolConfig(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array
	mock.StopInducingErrors()
	mock.SetValue([]byte(strconv.FormatBool(defaultValue)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Invalid byte array value
	mock.SetValue([]byte("cannot convert"))
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Shutdown the config via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}

func TestInt64Config(t *testing.T) {
	defaultValue := int64(math.MaxInt64)
	overridenValue := int64(math.MinInt64)
	mock := memory.NewConfig(nil)
	wrapper := NewInt64Config(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a positive value
	mock.SetValue([]byte(strconv.FormatInt(defaultValue, 10)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a negative value
	mock.SetValue([]byte(strconv.FormatInt(overridenValue, 10)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Invalid byte array value
	mock.SetValue([]byte("cannot convert"))
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Set the mock to an int
	mock.SetValue(42)
	val, err = wrapper.GetSafe(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, int64(42), val)
	assert.Equal(t, int64(42), wrapper.Get(context.Background()))

	// Shutdown via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}

func TestUint64Config(t *testing.T) {
	defaultValue := uint64(math.MaxUint64)
	overridenValue := uint64(0)
	mock := memory.NewConfig(nil)
	wrapper := NewUint64Config(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array
	mock.SetValue([]byte(strconv.FormatUint(defaultValue, 10)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Invalid byte array value
	mock.SetValue([]byte("cannot convert"))
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Set the mock to a uint
	mock.SetValue(uint(42))
	val, err = wrapper.GetSafe(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, uint64(42), val)
	assert.Equal(t, uint64(42), wrapper.Get(context.Background()))

	// Shutdown via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}
func TestFloat64Config(t *testing.T) {
	defaultValue := float64(math.Pi)
	overridenValue := float64(-math.Phi)
	mock := memory.NewConfig(nil)
	wrapper := NewFloat64Config(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a positive value
	mock.SetValue([]byte(strconv.FormatFloat(defaultValue, 'f', -1, 64)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a negative value
	mock.SetValue([]byte(strconv.FormatFloat(overridenValue, 'f', -1, 64)))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Invalid byte array value
	mock.SetValue([]byte("cannot convert"))
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Shutdown via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}

func TestStringConfig(t *testing.T) {
	defaultValue := "default"
	overridenValue := "override"
	mock := memory.NewConfig(nil)
	wrapper := NewStringConfig(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array
	mock.SetValue([]byte(defaultValue))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue(1234)
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Shutdown via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}

func TestDurationConfig(t *testing.T) {
	defaultValue := 30 * time.Second
	overridenValue := -2 * time.Hour
	mock := memory.NewConfig(nil)
	wrapper := NewDurationConfig(mock, defaultValue)

	// Return the default value when no override is set
	val, err := wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// The overriden value is returned when set
	mock.SetValue(overridenValue)
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The last observed config value is returned on error
	mock.InduceErrors()
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// The default value is returned when the override no longer has a value
	mock.StopInducingErrors()
	mock.ClearValue()
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a positive value
	mock.SetValue([]byte(defaultValue.String()))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, defaultValue, val)
	assert.Equal(t, defaultValue, wrapper.Get(context.Background()))

	// Verify conversion from a byte array with a negative value
	mock.SetValue([]byte(overridenValue.String()))
	val, err = wrapper.GetSafe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Invalid byte array value
	mock.SetValue([]byte("cannot convert"))
	val, err = wrapper.GetSafe(context.Background())
	require.Error(t, err)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Return an unsupported source value type
	mock.SetValue("not supported")
	val, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, err, ErrUnsuportedConversion)
	assert.Equal(t, overridenValue, val)
	assert.Equal(t, overridenValue, wrapper.Get(context.Background()))

	// Shutdown via the wrapper
	wrapper.Shutdown()
	_, err = wrapper.GetSafe(context.Background())
	assert.Equal(t, config.ErrShutdown, err)
}
