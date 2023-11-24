package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeviceTypeFromString(t *testing.T) {
	for _, value := range []string{
		"iOS",
		"ios",
	} {
		assert.Equal(t, DeviceTypeIOS, deviceTypeFromString(value))
	}

	for _, value := range []string{
		"Android",
		"android",
	} {
		assert.Equal(t, DeviceTypeAndroid, deviceTypeFromString(value))
	}

	assert.Equal(t, DeviceTypeUnknown, deviceTypeFromString("windows"))
}
