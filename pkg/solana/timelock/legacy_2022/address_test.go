package timelock_token

import (
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// todo: Base these on real-world examples from mainnet

func TestGetStateAddress(t *testing.T) {
	address, _, err := GetStateAddress(&GetStateAddressArgs{
		Mint:           mustBase58Decode("2U7X7s9CjKzkB9AqtJ4Am6hqiFhzPzisdEo3uMwuoPrq"),
		TimeAuthority:  mustBase58Decode("DpnXnwD9DcBnMxpbNM36cko3m41qgrsopqTNsUesMuyd"),
		Nonce:          mustBase58Decode("11111111111111111111111111111111"),
		VaultOwner:     mustBase58Decode("9N3Ypqi8FwNBK21bFbpysVqTupiH13JRQg9wQ4xnjYxK"),
		UnlockDuration: 1814400,
	})
	require.NoError(t, err)
	assert.Equal(t, "7BJrsePGaFkkYWuzy63VGy7N5ZA9Gou1BZHETCPRydiY", base58.Encode(address))
}

func TestGetVaultAddress(t *testing.T) {
	address, _, err := GetVaultAddress(&GetVaultAddressArgs{
		State: mustBase58Decode("7BJrsePGaFkkYWuzy63VGy7N5ZA9Gou1BZHETCPRydiY"),
	})
	require.NoError(t, err)
	assert.Equal(t, "9e6MBQRfQLQmaheuvixLvsTtZfPMw4L4BX2x9LTj2kf3", base58.Encode(address))
}
