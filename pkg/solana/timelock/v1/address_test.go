package timelock_token

import (
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests based on account created here https://explorer.solana.com/tx/2TTGdkrxuzQXe227CukjRMbppELELWyip9HQB2swuttgtfQovVp7bAXxSpBQjZpMpEWh17UFrwU1EYwiqYX5wK3

func TestGetStateAddress(t *testing.T) {
	address, _, err := GetStateAddress(&GetStateAddressArgs{
		Mint:          mustBase58Decode("kinXdEcpDQeHPEuQnqmUgtYykqKGVFq6CeVX5iAHJq6"),
		TimeAuthority: mustBase58Decode("codeHy87wGD5oMRLG75qKqsSi1vWE3oxNyYmXo5F9YR"),
		VaultOwner:    mustBase58Decode("BuAprBZugjXG6QRbRQN8QKF8EzbW5SigkDuyR9KtqN5z"),
		NumDaysLocked: DefaultNumDaysLocked,
	})
	require.NoError(t, err)
	assert.Equal(t, "7Ema8Z4gAUWegampp2AuX4cvaTRy3VMwJUq8LMJshQTV", base58.Encode(address))
}

func TestGetVaultAddress(t *testing.T) {
	address, _, err := GetVaultAddress(&GetVaultAddressArgs{
		State:       mustBase58Decode("7Ema8Z4gAUWegampp2AuX4cvaTRy3VMwJUq8LMJshQTV"),
		DataVersion: DataVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, "3538bYdWoRXUgBbyAyvG3Zemmawh75nmCQEvWc9DfKFR", base58.Encode(address))
}
