package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrToQuarks_HappyPath(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected int64
	}{
		{"123.456", 123456000},
		{"123", 123000000},
		{"0.456", 456000},
		{"1234567890123.123456", 1234567890123123456},
	} {
		quarks, err := StrToQuarks(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, quarks)
	}
}

func TestStrToQuarks_InvalidString(t *testing.T) {
	for _, tc := range []string{
		"",
		"abc",
		"1.1.1",
		"0.1234567",
		"12345678901234",
	} {
		_, err := StrToQuarks(tc)
		assert.Error(t, err)
	}
}
