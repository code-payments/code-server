package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Required because we'd have a dependency loop with the testutil package
func newRandomTestAccount(t *testing.T) *Account {
	account, err := NewRandomAccount()
	require.NoError(t, err)
	return account
}
