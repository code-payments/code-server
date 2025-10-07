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

// Required because we'd have a dependency loop with the testutil package
func newRandomVmConfig(t *testing.T, isCore bool) *VmConfig {
	if isCore {
		return &VmConfig{
			Authority: newRandomTestAccount(t),
			Vm:        newRandomTestAccount(t),
			Omnibus:   newRandomTestAccount(t),
			Mint:      CoreMintAccount,
		}
	}
	return &VmConfig{
		Authority: newRandomTestAccount(t),
		Vm:        newRandomTestAccount(t),
		Omnibus:   newRandomTestAccount(t),
		Mint:      newRandomTestAccount(t),
	}
}
