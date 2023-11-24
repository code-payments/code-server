package thirdparty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/testutil"
)

func TestNaclBoxBlockchainMessage_RoundTrip(t *testing.T) {
	sender := testutil.NewRandomAccount(t)
	receiver := testutil.NewRandomAccount(t)

	senderDomain := "app.getcode.com"
	plaintextMessage := "super secret message"

	blockchainMessage, err := NewNaclBoxBlockchainMessage(
		senderDomain,
		plaintextMessage,
		sender,
		receiver,
	)
	require.NoError(t, err)

	assert.Equal(t, NaclBoxBlockchainMessage, blockchainMessage.Type)
	assert.EqualValues(t, 0, blockchainMessage.Version)
	assert.EqualValues(t, 0, blockchainMessage.Flags)
	assert.Equal(t, senderDomain, blockchainMessage.SenderDomain)
	assert.Equal(t, receiver.PublicKey().ToBase58(), blockchainMessage.ReceiverAccount.PublicKey().ToBase58())
	assert.Len(t, blockchainMessage.Nonce, naclBoxNonceLength)
	assert.NotEqual(t, naclBoxNonce{}, blockchainMessage.Nonce)
	assert.NotEqual(t, plaintextMessage, string(blockchainMessage.EncryptedMessage))

	encodedBytes, err := blockchainMessage.Encode()
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(encodedBytes), plaintextMessage))

	decodedBlockchainMessage, err := DecodeBlockchainMessage(encodedBytes)
	require.NoError(t, err)

	assert.Equal(t, blockchainMessage.Type, decodedBlockchainMessage.Type)
	assert.Equal(t, blockchainMessage.Version, decodedBlockchainMessage.Version)
	assert.Equal(t, blockchainMessage.Flags, decodedBlockchainMessage.Flags)
	assert.Equal(t, blockchainMessage.SenderDomain, decodedBlockchainMessage.SenderDomain)
	assert.Equal(t, blockchainMessage.ReceiverAccount.PublicKey().ToBase58(), decodedBlockchainMessage.ReceiverAccount.PublicKey().ToBase58())
	assert.Equal(t, blockchainMessage.Nonce, decodedBlockchainMessage.Nonce)
	assert.Equal(t, blockchainMessage.EncryptedMessage, decodedBlockchainMessage.EncryptedMessage)

	decrypted, err := decryptMessageUsingNaclBox(receiver, sender, decodedBlockchainMessage.EncryptedMessage, [24]byte(decodedBlockchainMessage.Nonce))
	require.NoError(t, err)
	assert.Equal(t, plaintextMessage, string(decrypted))
}
