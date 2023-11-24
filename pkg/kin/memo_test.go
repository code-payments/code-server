package kin

import (
	"encoding/base64"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemo_Valid(t *testing.T) {
	var emptyFK = make([]byte, 29)

	for v := byte(0); v <= 7; v++ {
		m, err := NewMemo(v, TransactionTypeEarn, 1, make([]byte, 29))
		require.NoError(t, err)

		require.EqualValues(t, magicByte, m[0]&0x3)
		require.EqualValues(t, v, m.Version())
		require.EqualValues(t, TransactionTypeEarn, m.TransactionType())
		require.EqualValues(t, 1, m.AppIndex())
		require.EqualValues(t, emptyFK, m.ForeignKey())
	}

	for txType := TransactionTypeNone; txType <= MaxTransactionType; txType++ {
		m, err := NewMemo(1, txType, 1, make([]byte, 29))
		require.NoError(t, err)

		require.EqualValues(t, magicByte, m[0]&0x3)
		require.EqualValues(t, 1, m.Version())
		require.EqualValues(t, txType, m.TransactionType())
		require.EqualValues(t, 1, m.AppIndex())
		require.EqualValues(t, emptyFK, m.ForeignKey())
	}

	for i := uint16(0); i < math.MaxUint16; i++ {
		m, err := NewMemo(1, TransactionTypeEarn, i, make([]byte, 29))
		require.NoError(t, err)

		require.EqualValues(t, magicByte, m[0]&0x3)
		require.EqualValues(t, 1, m.Version())
		require.EqualValues(t, TransactionTypeEarn, m.TransactionType())
		require.EqualValues(t, i, m.AppIndex())
		require.EqualValues(t, emptyFK, m.ForeignKey())
	}

	for i := 0; i < 256; i += 29 {
		fk := make([]byte, 29)
		for j := 0; j < 29; j++ {
			fk[j] = byte(i + j) // this eventually overflows, but that's ok
		}

		m, err := NewMemo(1, TransactionTypeEarn, 2, fk)
		require.NoError(t, err)

		actual := m.ForeignKey()
		for j := 0; j < 28; j++ {
			require.Equal(t, fk[j], actual[j])
		}

		// Note, because we only have 230 bits, the last byte in the memo fk
		// only has the first 6 bits of the last byte in the original fk.
		require.Equal(t, fk[28]&0x3f, actual[28])
	}

	// Test a short foreign key
	fk := []byte{byte(1), byte(255)}
	m, err := NewMemo(1, TransactionTypeEarn, 2, fk)
	require.NoError(t, err)

	actual := m.ForeignKey()
	require.Equal(t, fk, actual[:2])

	// Test no foreign key
	m, err = NewMemo(1, TransactionTypeEarn, 2, nil)
	require.NoError(t, err)

	actual = m.ForeignKey()
	require.Equal(t, make([]byte, 29), actual)
}

func TestMemo_TransactionTypeRaw(t *testing.T) {
	for i := 0; i < 32; i++ {
		m, err := NewMemo(1, TransactionType(i), 0, nil)
		require.NoError(t, err)
		require.Equal(t, m.TransactionTypeRaw(), TransactionType(i))
	}
}

func TestMemo_TransactionType(t *testing.T) {
	for txType := TransactionTypeNone; txType <= MaxTransactionType; txType++ {
		m, err := NewMemo(1, txType, 0, nil)
		require.NoError(t, err)
		require.Equal(t, m.TransactionType(), txType)
	}

	m, err := NewMemo(1, MaxTransactionType+1, 0, nil)
	require.NoError(t, err)
	require.Equal(t, m.TransactionType(), TransactionTypeUnknown)
}

func TestMemo_Invalid(t *testing.T) {
	// Invalid version
	_, err := NewMemo(8, TransactionTypeEarn, 1, make([]byte, 29))
	require.NotNil(t, err)

	m, err := NewMemo(1, TransactionTypeEarn, 1, make([]byte, 29))
	require.Nil(t, err)
	require.True(t, IsValidMemo(m))
	require.True(t, IsValidMemoStrict(m))

	// Invalid magic byte
	m[0] &= 0xfc
	require.False(t, IsValidMemo(m))
	require.False(t, IsValidMemoStrict(m))

	// Invalid transaction type
	_, err = NewMemo(1, TransactionTypeUnknown, 1, make([]byte, 29))
	require.NotNil(t, err)

	m, err = NewMemo(1, MaxTransactionType+1, 1, make([]byte, 29))
	require.Nil(t, err)
	require.True(t, IsValidMemo(m))
	require.False(t, IsValidMemoStrict(m))

	// Version higher than configured
	m, err = NewMemo(7, TransactionTypeEarn, 1, make([]byte, 29))
	require.Nil(t, err)
	require.True(t, IsValidMemo(m))
	require.False(t, IsValidMemoStrict(m))

	// Transaction type higher than configured
	m, err = NewMemo(1, MaxTransactionType+1, 1, make([]byte, 29))
	require.Nil(t, err)
	require.True(t, IsValidMemo(m))
	require.False(t, IsValidMemoStrict(m))
}

func TestMemoFromBase64(t *testing.T) {
	validMemo, _ := NewMemo(2, TransactionTypeEarn, 1, make([]byte, 29))
	actual, err := MemoFromBase64String(base64.StdEncoding.EncodeToString(validMemo[:]), false)
	require.NoError(t, err)
	require.Equal(t, validMemo, actual)

	_, err = MemoFromBase64String(base64.StdEncoding.EncodeToString(validMemo[:]), true)
	require.Error(t, err)

	strictlyValidMemo, _ := NewMemo(1, TransactionTypeEarn, 1, make([]byte, 29))
	actual, err = MemoFromBase64String(base64.StdEncoding.EncodeToString(strictlyValidMemo[:]), false)
	require.NoError(t, err)
	require.Equal(t, strictlyValidMemo, actual)

	actual, err = MemoFromBase64String(base64.StdEncoding.EncodeToString(strictlyValidMemo[:]), true)
	require.NoError(t, err)
	require.Equal(t, strictlyValidMemo, actual)

	invalidMemos := []string{
		"somememo",
		base64.StdEncoding.EncodeToString([]byte("somememo")),
	}
	for _, m := range invalidMemos {
		_, err := MemoFromBase64String(m, false)
		require.Error(t, err)
	}
}
