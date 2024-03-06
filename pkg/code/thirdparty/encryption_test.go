package thirdparty

import (
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	"github.com/code-payments/code-server/pkg/code/common"
)

func TestNaclBox_Ed25519ToCurve25519(t *testing.T) {
	account, err := common.NewAccountFromPrivateKeyString("4vXZTu7W8FKV2cNB7t2MTp8KXrWpJRCodzUPoyPy1MWZiZQqVVXUrycCdoagzPN6YE9w9pyTbZVzVw9iLDUT7adR")
	require.NoError(t, err)

	curve25519PrivateKey := ed25519ToCurve25519PrivateKey(account)
	assert.Equal(t, "F197LA9gxNFgu6bwmHFuBJWU4yuA3wRsBDky9twjeoJr", base58.Encode(curve25519PrivateKey[:]))

	curve25519PublicKey := ed25519ToCurve25519PublicKey(account)
	assert.Equal(t, "nRwUgVGq7NUAodgaByFjTXPRAWcT6W3dtaphzWTq7tp", base58.Encode(curve25519PublicKey[:]))
}

func TestNaclBox_SharedKey(t *testing.T) {
	account1, err := common.NewAccountFromPrivateKeyString("2fJLfaTREkNBiDbB26dL4syDozhCEf2pNMorXvBf7593yC59d1kDFsXAA9cN63Bb5MDUgSeU5AhsfS2aTZQHoNyU")
	require.NoError(t, err)

	account2, err := common.NewAccountFromPrivateKeyString("3GKRCGo814rSVa6XkFARZGq13Rb7DSGwF2c6SSRSzMfyQ3wuDAPoELzhsvH6r5A1PFACpFuesDaRHUEoL1PFAxRa")
	require.NoError(t, err)

	curve25519PrivateKey1 := ed25519ToCurve25519PrivateKey(account1)
	curve25519PublicKey1 := ed25519ToCurve25519PublicKey(account1)

	curve25519PrivateKey2 := ed25519ToCurve25519PrivateKey(account2)
	curve25519PublicKey2 := ed25519ToCurve25519PublicKey(account2)

	var shared [32]byte
	box.Precompute(&shared, &curve25519PublicKey1, &curve25519PrivateKey2)
	assert.Equal(t, "GC1cihUsj3rBqqdzBmWkEejWuv6p3scxPqCEwUBUUdQq", base58.Encode(shared[:]))

	box.Precompute(&shared, &curve25519PublicKey2, &curve25519PrivateKey1)
	assert.Equal(t, "GC1cihUsj3rBqqdzBmWkEejWuv6p3scxPqCEwUBUUdQq", base58.Encode(shared[:]))
}

func TestNaclBox_RoundTrip(t *testing.T) {
	plaintextMessage := "super secret message"

	sender, err := common.NewAccountFromPrivateKeyString("2tKSW5f1dag1pGzDSsM9yo32KSMNcTkBAvXEfZ1u2pcqkmo8oYcbtsnA8m9YVd8EUzVJeU5mvjFKjPQF2m4Xifg8")
	require.NoError(t, err)

	receiver, err := common.NewAccountFromPrivateKeyString("38EyWg6Eay5bhcZR465FD2agT2bf7BhyWNJJ64ypfdQGTb6mHU3an2f8pvWapSrE3j3hEFu1h7HYoa6eykAHUBJr")
	require.NoError(t, err)

	var nonce naclBoxNonce
	decodedNonce, err := base58.Decode("Jc1X8GdaMmcRDRKiAaMZSRBDLZAFuf9xq")
	require.NoError(t, err)
	require.Len(t, decodedNonce, len(nonce))
	copy(nonce[:], decodedNonce)

	expectedEncryptedValue := "2eXsYDo1gcuYc1Nw7uUGZmJZrj2vu33TnrXve62HwzhyTggjjz"
	actualEncryptedValue := encryptMessageUsingNaclBoxWithProvidedNonce(sender, receiver, plaintextMessage, nonce)
	assert.Equal(t, expectedEncryptedValue, base58.Encode(actualEncryptedValue))

	decryptedValue, err := decryptMessageUsingNaclBox(sender, receiver, actualEncryptedValue, nonce)
	require.NoError(t, err)
	assert.Equal(t, plaintextMessage, decryptedValue)
}
