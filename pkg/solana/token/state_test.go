package token

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshal(t *testing.T) {
	data, err := hex.DecodeString("118a08c9d4cc46c576282e0daf050bbdb04f03313e35e5db3f3def69fa1eeec42b15a9cd4bef2cd809e464570d2a6cbd9bcc64e32ea4ebbcf748757bbb3dd5bd000084e2506ce67c000000000000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)

	mint, err := base58.Decode("2BU1Xgyzqixhjaq9Pa5cNsaa1gSejLeNtDaDRv29qoZm")
	require.NoError(t, err)

	var a Account
	require.True(t, a.Unmarshal(data))
	assert.Equal(t, mint, []byte(a.Mint))
	assert.Equal(t, uint64(9e13*1e5), a.Amount)
	assert.Empty(t, a.Delegate)
	assert.Empty(t, a.CloseAuthority)

	var rtt Account
	rtt.Unmarshal(a.Marshal())
	assert.Equal(t, a, rtt)
}

func TestRoundTrip(t *testing.T) {
	mint := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for i := 0; i < len(mint); i++ {
		mint[i] = 1
	}
	owner := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for i := 0; i < len(owner); i++ {
		owner[i] = 2
	}
	delegate := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for i := 0; i < len(delegate); i++ {
		delegate[i] = 3
	}
	closeAuthority := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for i := 0; i < len(closeAuthority); i++ {
		closeAuthority[i] = 2
	}

	isNative := uint64(2)
	expected := Account{
		Mint:           mint,
		Owner:          owner,
		Amount:         10,
		Delegate:       delegate,
		State:          AccountStateFrozen,
		IsNative:       &isNative,
		CloseAuthority: closeAuthority,
	}

	var actual Account
	require.True(t, actual.Unmarshal(expected.Marshal()))
	assert.Equal(t, expected, actual)
}
