package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodingRoundTrip(t *testing.T) {
	key := []byte{
		0x6D, 0x70, 0x72, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x40, 0x71,
		0xD8, 0x9E, 0x81, 0x34, 0x63,
		0x06, 0xA0, 0x35, 0xA6, 0x83,
	}
	encoded, err := Encode(key)
	require.NoError(t, err)

	decoded, err := Decode(encoded)
	require.NoError(t, err)

	assert.EqualValues(t, key, decoded)
}
