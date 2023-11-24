package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AssertStatusErrorWithCode verifies that the provided error is a gRPC status
// error of the provided status code.
func AssertStatusErrorWithCode(t *testing.T, err error, code codes.Code) {
	require.Error(t, err)
	status, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, code, status.Code())
}
