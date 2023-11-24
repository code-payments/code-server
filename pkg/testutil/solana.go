package testutil

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
)

func GenerateSolanaKeypair(t *testing.T) ed25519.PrivateKey {
	_, p, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return p
}

func GenerateSolanaKeys(t *testing.T, n int) []ed25519.PublicKey {
	keys := make([]ed25519.PublicKey, n)
	for i := 0; i < n; i++ {
		p, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys[i] = p
	}
	return keys
}

func NewRandomAccount(t *testing.T) *common.Account {
	account, err := common.NewRandomAccount()
	require.NoError(t, err)

	return account
}
