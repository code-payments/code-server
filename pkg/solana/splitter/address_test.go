package splitter_token

import (
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pool tests based on account created here https://explorer.solana.com/tx/5vsH81D96mXTmKMiTSzDhM2nRmbvPr6CtcaWMbz21t4QWnCji8zPGt9ZBBkzJvoGJcipR3cp1fTT8CmnGRwrkmHq
// Commitment tests based on account created here https://explorer.solana.com/tx/3tBhdHRsq5dBKHxY4fz8uiUAqxp7B9RSmEijFHCTW68aLTwG1Por1NPMtCxaqdPWrdFWtrtfaSwpBoXPkvLYgSyr
// Proof tests based on account created here https://explorer.solana.com/tx/3Q27PZ8ZtaDChGVJSpx667CzQH7Xwh5W6qbnHgWbb6roE37doNGc4LgTzTdi2GcnJZti8YaPmYxVodPbpgTtHm8E

func TestGetPoolStateAddress(t *testing.T) {
	address, _, err := GetPoolStateAddress(&GetPoolStateAddressArgs{
		Mint:      mustBase58Decode("kinXdEcpDQeHPEuQnqmUgtYykqKGVFq6CeVX5iAHJq6"),
		Authority: mustBase58Decode("codeHy87wGD5oMRLG75qKqsSi1vWE3oxNyYmXo5F9YR"),
		Name:      "codedev-treasury-3",
	})
	require.NoError(t, err)
	assert.Equal(t, "3HR2k4etyHtBgHCAisRQ5mAU1x3GxWSgmm1bHsNzvZKS", base58.Encode(address))
}

func TestGetPoolVaultAddress(t *testing.T) {
	address, _, err := GetPoolVaultAddress(&GetPoolVaultAddressArgs{
		Pool: mustBase58Decode("3HR2k4etyHtBgHCAisRQ5mAU1x3GxWSgmm1bHsNzvZKS"),
	})
	require.NoError(t, err)
	assert.Equal(t, "BF6vGtAGf1WWbgyKQeHpyiieRKM6X3jZm8QJYib7e3XV", base58.Encode(address))
}

func TestGetCommitmentStateAddress(t *testing.T) {
	address, _, err := GetCommitmentStateAddress(&GetCommitmentStateAddressArgs{
		Pool:        mustBase58Decode("3HR2k4etyHtBgHCAisRQ5mAU1x3GxWSgmm1bHsNzvZKS"),
		RecentRoot:  mustHexDecode("a26336ee808c6da0efa8f9977987aca89b61557c2de7090f0e092a500dbdc9b4"),
		Transcript:  mustHexDecode("7703412617e79c889e0fade0ddc3ac27ce823d7c82ec4062284c9c82bd36346f"),
		Destination: mustBase58Decode("A1WsiTaL6fPei2xcqDPiVnRDvRwpCjne3votXZmrQe86"),
		Amount:      100000,
	})
	require.NoError(t, err)
	assert.Equal(t, "4vF8wWhuUSPTmUWPRvNcB5aPNzDvjCYBhyizpG6VFNi6", base58.Encode(address))
}

func TestGetCommitmentVaultAddress(t *testing.T) {
	address, _, err := GetCommitmentVaultAddress(&GetCommitmentVaultAddressArgs{
		Pool:       mustBase58Decode("3HR2k4etyHtBgHCAisRQ5mAU1x3GxWSgmm1bHsNzvZKS"),
		Commitment: mustBase58Decode("4vF8wWhuUSPTmUWPRvNcB5aPNzDvjCYBhyizpG6VFNi6"),
	})
	require.NoError(t, err)
	assert.Equal(t, "7BXkxmuwH4GGm48gPWMWqHnLYX7NwrtGPUtfHKnhgMmZ", base58.Encode(address))
}

func TestGetProofAddress(t *testing.T) {
	address, _, err := GetProofAddress(&GetProofAddressArgs{
		Pool:       mustBase58Decode("3HR2k4etyHtBgHCAisRQ5mAU1x3GxWSgmm1bHsNzvZKS"),
		MerkleRoot: mustHexDecode("e8eb6f7266f0de778923e5cdadf30c3d8a4c11a518c40fc96c65acdc4fd3bb2b"),
		Commitment: mustBase58Decode("4vF8wWhuUSPTmUWPRvNcB5aPNzDvjCYBhyizpG6VFNi6"),
	})
	require.NoError(t, err)
	assert.Equal(t, "9VNbqqrDVAxCPmXTq1BDWw7b3gjcXcwn9j2hAhsHWdNB", base58.Encode(address))
}
