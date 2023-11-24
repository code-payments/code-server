package shortvec

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortVec_Valid(t *testing.T) {
	for i := 0; i < math.MaxUint16; i++ {
		buf := &bytes.Buffer{}
		_, err := EncodeLen(buf, i)
		require.NoError(t, err)

		actual, err := DecodeLen(buf)
		require.NoError(t, err)
		require.Equal(t, i, actual)
	}
}

func TestShortVec_CrossImpl(t *testing.T) {
	for _, tc := range []struct {
		val     int
		encoded []byte
	}{
		{0x0, []byte{0x0}},
		{0x7f, []byte{0x7f}},
		{0x80, []byte{0x80, 0x01}},
		{0xff, []byte{0xff, 0x01}},
		{0x100, []byte{0x80, 0x02}},
		{0x7fff, []byte{0xff, 0xff, 0x01}},
		{0xffff, []byte{0xff, 0xff, 0x03}},
	} {
		buf := &bytes.Buffer{}
		n, err := EncodeLen(buf, tc.val)
		require.NoError(t, err)
		assert.Equal(t, len(tc.encoded), n)
		assert.Equal(t, tc.encoded, buf.Bytes())
	}
}

func TestShortVec_Invalid(t *testing.T) {
	_, err := EncodeLen(&bytes.Buffer{}, math.MaxUint16+1)
	require.NotNil(t, err)
}
