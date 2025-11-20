package client

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/grpc/headers"
)

func TestGetUserAgent_HappyPath(t *testing.T) {
	for _, headerValue := range []string{
		"Code/iOS/11.22.33",
		"Code/Android/11.22.33",
		"OpenCodeProtocol/Android/11.22.33",
		"Mozilla/5.0 Code/iOS/11.22.33 Mobile Safari/533.1",
	} {
		ctx := context.Background()
		ctx, err := headers.ContextWithHeaders(ctx)
		require.NoError(t, err)
		require.NoError(t, headers.SetASCIIHeader(ctx, UserAgentHeaderName, headerValue))

		userAgent, err := GetUserAgent(ctx)
		require.NoError(t, err)

		if strings.Contains(headerValue, DeviceTypeIOS.String()) {
			assert.Equal(t, DeviceTypeIOS, userAgent.DeviceType)
		} else {
			assert.Equal(t, DeviceTypeAndroid, userAgent.DeviceType)
		}

		assert.Equal(t, 11, userAgent.Version.Major)
		assert.Equal(t, 22, userAgent.Version.Minor)
		assert.Equal(t, 33, userAgent.Version.Patch)
	}
}

func TestGetUserAgent_ParseError(t *testing.T) {
	for _, headerValue := range []string{
		// No Code value
		"Mozilla/5.0 Mobile Safari/533.1",

		// Unsupported device type
		"Code/Windows/1.2.3",

		// Version components missing
		"Code/iOS/.2.3",
		"Code/iOS/..3",
		"Code/iOS/..",
		"Code/iOS/",
		"Code/iOS",
	} {
		ctx := context.Background()
		ctx, err := headers.ContextWithHeaders(ctx)
		require.NoError(t, err)
		require.NoError(t, headers.SetASCIIHeader(ctx, UserAgentHeaderName, headerValue))

		_, err = GetUserAgent(ctx)
		assert.Error(t, err)
	}
}

func TestUserAgent_StringValue(t *testing.T) {
	ua := UserAgent{
		DeviceType: DeviceTypeIOS,
		Version: Version{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}

	assert.Equal(t, "OpenCodeProtocol/iOS/1.2.3", ua.String())
}
