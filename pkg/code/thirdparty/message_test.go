package thirdparty

import (
	"strings"
	"testing"

	"github.com/google/uuid"
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

	decodedBlockchainMessage, err := DecodeNaclBoxBlockchainMessage(encodedBytes)
	require.NoError(t, err)

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

func TestFiatOnrampPurchaseMessage_RoundTrip(t *testing.T) {
	nonce := uuid.New()

	blockchainMessage, err := NewFiatOnrampPurchaseMessage(nonce)
	require.NoError(t, err)

	assert.EqualValues(t, 0, blockchainMessage.Version)
	assert.EqualValues(t, 0, blockchainMessage.Flags)
	assert.Equal(t, nonce, blockchainMessage.Nonce)

	encodedBytes, err := blockchainMessage.Encode()
	require.NoError(t, err)

	decodedBlockchainMessage, err := DecodeFiatOnrampPurchaseMessage(encodedBytes)
	require.NoError(t, err)

	assert.Equal(t, blockchainMessage.Version, decodedBlockchainMessage.Version)
	assert.Equal(t, blockchainMessage.Flags, decodedBlockchainMessage.Flags)
	assert.Equal(t, blockchainMessage.Nonce, decodedBlockchainMessage.Nonce)
}

func TestFiatOnrampPurchase_CrossPlatform(t *testing.T) {
	nonce, err := uuid.Parse("c24a3bf2-ad4f-4756-944e-81948ff10882")
	require.NoError(t, err)

	msg, err := NewFiatOnrampPurchaseMessage(nonce)
	require.NoError(t, err)

	expected := "AQAAAAAAwko78q1PR1aUToGUj/EIgg=="
	actual, err := msg.Encode()
	require.NoError(t, err)
	assert.Equal(t, expected, string(actual))
}
