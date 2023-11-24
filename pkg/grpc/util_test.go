package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFullMethodName_ComponentExtraction(t *testing.T) {
	packageName, serviceName, methodName, err := ParseFullMethodName("/internal.v1.test.Service/Method")
	require.NoError(t, err)

	assert.Equal(t, "internal.v1.test", packageName)
	assert.Equal(t, "Service", serviceName)
	assert.Equal(t, "Method", methodName)
}

func TestParseFullMethodName_Validation(t *testing.T) {
	for _, invalidValue := range []string{
		"",
		"/internal.v1.test.Service",
		"/internal.v1.test.Service/",
		"internal.v1.test.Service/Method",
		"/internal.v1.test.Service./Method",
		"/Service/Method",
	} {
		_, _, _, err := ParseFullMethodName(invalidValue)
		assert.Error(t, err)
	}
}
