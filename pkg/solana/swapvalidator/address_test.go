package swap_validator

import (
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests based on account created here https://solscan.io/tx/4kXywqQRqJXRnm8mzAVRYkL1YVwyAEi8rVVNzRYG3AAu73xqjxNdHKX6LgoFhon66ZWHJ8HU3QkAmiot7AGzXPzd

func TestPreSwapStateAddress(t *testing.T) {
	address, _, err := GetPreSwapStateAddress(&GetPreSwapStateAddressArgs{
		Source:      mustBase58Decode("5nNBW1KhzHVbR4NMPLYPRYj3UN5vgiw5GrtpdK6eGoce"),
		Destination: mustBase58Decode("9Rgx4kjnYZBbeXXgbbYLT2FfgzrNHFUShDtp8dpHHjd2"),
		Nonce:       mustBase58Decode("3SVPEF5HDcKLhVfKeAnbH5Azpyeuk2yyVjEjZbz4VhrL"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Hh338LHJhkzPbDisGt5Lge8qkgc3RExvH7BdmKgnRQw9", base58.Encode(address))
}
