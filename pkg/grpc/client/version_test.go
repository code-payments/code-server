package client

import (
	"testing"

	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func TestVersion_ParseHappyPath(t *testing.T) {
	version, err := ParseVersion(" 11.22.33 ")
	require.NoError(t, err)
	assert.Equal(t, 11, version.Major)
	assert.Equal(t, 22, version.Minor)
	assert.Equal(t, 33, version.Patch)

	version, err = ParseVersion("1.2")
	require.NoError(t, err)
	assert.Equal(t, 1, version.Major)
	assert.Equal(t, 2, version.Minor)
	assert.Equal(t, 0, version.Patch)
}

func TestVersion_ParseError(t *testing.T) {
	for _, value := range []string{
		// Version components missing
		"1",
		"1.",
		"1.2.",
		".2.3",
		"..3",
		"..",

		// Non-digit values
		"a.2.3",
		"1.a.3",
		"2.a.3",

		// Negative values in verion
		"-1.2.3",
		"1.-2.3",
		"1.2.-3",

		// Whitespace in version
		"1 .2.3",
		"1. 2.3",
		"1.2 .3",
		"1.2. 3",
	} {
		_, err := ParseVersion(value)
		assert.Error(t, err)
	}
}

func TestVersion_StringValue(t *testing.T) {
	version := Version{
		Major: 1,
		Minor: 2,
		Patch: 3,
	}

	assert.Equal(t, "1.2.3", version.String())
}

func TestVersion_Comparison(t *testing.T) {
	baseline := &Version{
		Major: 1,
		Minor: 2,
		Patch: 3,
	}

	assert.True(t, baseline.GreaterThanOrEqualTo(baseline))
	assert.False(t, baseline.Before(baseline))

	patchAfterBaseline := &Version{
		Major: baseline.Major,
		Minor: baseline.Minor,
		Patch: baseline.Patch + 1,
	}
	assert.False(t, baseline.GreaterThanOrEqualTo(patchAfterBaseline))
	assert.True(t, baseline.Before(patchAfterBaseline))

	patchBeforeBaseline := &Version{
		Major: baseline.Major,
		Minor: baseline.Minor,
		Patch: baseline.Patch - 1,
	}
	assert.True(t, baseline.GreaterThanOrEqualTo(patchBeforeBaseline))
	assert.False(t, baseline.Before(patchBeforeBaseline))

	for _, patch := range []int{
		baseline.Patch,
		baseline.Patch + 1,
		baseline.Patch - 1,
	} {
		minorAfterBaseline := &Version{
			Major: baseline.Major,
			Minor: baseline.Minor + 1,
			Patch: patch,
		}
		assert.False(t, baseline.GreaterThanOrEqualTo(minorAfterBaseline))
		assert.True(t, baseline.Before(minorAfterBaseline))

		minorBeforeBaseline := &Version{
			Major: baseline.Major,
			Minor: baseline.Minor - 1,
			Patch: patch,
		}
		assert.True(t, baseline.GreaterThanOrEqualTo(minorBeforeBaseline))
		assert.False(t, baseline.Before(minorBeforeBaseline))
	}

	for _, minor := range []int{
		baseline.Minor,
		baseline.Minor + 1,
		baseline.Minor - 1,
	} {
		for _, patch := range []int{
			baseline.Patch,
			baseline.Patch + 1,
			baseline.Patch - 1,
		} {
			majorAfterBaseline := &Version{
				Major: baseline.Major + 1,
				Minor: minor,
				Patch: patch,
			}
			assert.False(t, baseline.GreaterThanOrEqualTo(majorAfterBaseline))
			assert.True(t, baseline.Before(majorAfterBaseline))

			majorBeforeBaseline := &Version{
				Major: baseline.Major - 1,
				Minor: minor,
				Patch: patch,
			}
			assert.True(t, baseline.GreaterThanOrEqualTo(majorBeforeBaseline))
			assert.False(t, baseline.Before(majorBeforeBaseline))
		}
	}
}

func TestCheckMinVersion(t *testing.T) {
	minVersionByDevice[DeviceTypeIOS] = &Version{
		Major: 2,
		Minor: 0,
		Patch: 0,
	}
	minVersionByDevice[DeviceTypeAndroid] = &Version{
		Major: 1,
		Minor: 0,
		Patch: 0,
	}

	// Version for device exceeds the minimum
	userAgent := &UserAgent{
		DeviceType: DeviceTypeIOS,
		Version: Version{
			Major: 3,
			Minor: 0,
			Patch: 0,
		},
	}
	assert.NoError(t, checkMinVersion(userAgent))

	// Version for device is the minimum
	userAgent = &UserAgent{
		DeviceType: DeviceTypeIOS,
		Version: Version{
			Major: 2,
			Minor: 0,
			Patch: 0,
		},
	}
	assert.NoError(t, checkMinVersion(userAgent))

	// Version for device is below the minimum
	userAgent = &UserAgent{
		DeviceType: DeviceTypeIOS,
		Version: Version{
			Major: 1,
			Minor: 0,
			Patch: 0,
		},
	}
	testutil.AssertStatusErrorWithCode(t, checkMinVersion(userAgent), codes.FailedPrecondition)

	// Unknown devices fails min version check
	userAgent = &UserAgent{
		DeviceType: DeviceTypeUnknown,
		Version: Version{
			Major: 10,
			Minor: 0,
			Patch: 0,
		},
	}
	testutil.AssertStatusErrorWithCode(t, checkMinVersion(userAgent), codes.FailedPrecondition)
}
