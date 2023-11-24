package common

import (
	"crypto/ed25519"
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicKey(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	var keys []*Key

	key, err := NewKeyFromBytes(publicKey)
	require.NoError(t, err)
	keys = append(keys, key)

	key, err = NewKeyFromString(base58.Encode(publicKey))
	require.NoError(t, err)
	keys = append(keys, key)

	for _, key := range keys {
		assert.True(t, key.IsPublic())
		assert.EqualValues(t, publicKey, key.ToBytes())
		assert.Equal(t, base58.Encode(publicKey), key.ToBase58())
	}
}

func TestPrivateKey(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	var keys []*Key

	key, err := NewKeyFromBytes(privateKey)
	require.NoError(t, err)
	keys = append(keys, key)

	key, err = NewKeyFromString(base58.Encode(privateKey))
	require.NoError(t, err)
	keys = append(keys, key)

	for _, key := range keys {
		assert.False(t, key.IsPublic())
		assert.EqualValues(t, privateKey, key.ToBytes())
		assert.Equal(t, base58.Encode(privateKey), key.ToBase58())
	}
}

func TestInvalidKey(t *testing.T) {
	stringValue := "invalid-key"
	bytesValue := []byte(stringValue)

	_, err := NewKeyFromString(stringValue)
	assert.Error(t, err)

	_, err = NewKeyFromBytes(bytesValue)
	assert.Error(t, err)
}
