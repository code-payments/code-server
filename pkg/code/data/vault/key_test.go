package vault_test

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateKey(t *testing.T) {
	for i := 0; i < 100; i++ {
		actual, err := vault.CreateKey()
		require.NoError(t, err)
		assert.Equal(t, vault.StateAvailable, actual.State)

		_, err = base58.Decode(actual.PublicKey)
		require.NoError(t, err)

		_, err = base58.Decode(actual.PrivateKey)
		require.NoError(t, err)
	}
}

func TestGrindKey(t *testing.T) {
	for i := 0; i < 5; i++ {
		actual, err := vault.GrindKey("x")
		require.NoError(t, err)
		assert.Equal(t, vault.StateAvailable, actual.State)
		assert.EqualValues(t, 'x', actual.PublicKey[0])

		_, err = base58.Decode(actual.PublicKey)
		require.NoError(t, err)

		_, err = base58.Decode(actual.PrivateKey)
		require.NoError(t, err)
	}

	for i := 0; i < 5; i++ {
		actual, err := vault.GrindKey("y")
		require.NoError(t, err)
		assert.Equal(t, vault.StateAvailable, actual.State)
		assert.EqualValues(t, 'y', actual.PublicKey[0])

		_, err = base58.Decode(actual.PublicKey)
		require.NoError(t, err)

		_, err = base58.Decode(actual.PrivateKey)
		require.NoError(t, err)
	}

	for i := 0; i < 5; i++ {
		actual, err := vault.GrindKey("z")
		require.NoError(t, err)
		assert.Equal(t, vault.StateAvailable, actual.State)
		assert.EqualValues(t, 'z', actual.PublicKey[0])

		_, err = base58.Decode(actual.PublicKey)
		require.NoError(t, err)

		_, err = base58.Decode(actual.PrivateKey)
		require.NoError(t, err)
	}
}
