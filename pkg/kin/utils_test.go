package kin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKinToQuarks(t *testing.T) {
	validCases := map[string]int64{
		"0.00001": 1,
		"0.00002": 2,
		"0.00020": 20,
		"0.00200": 200,
		"0.02000": 2000,
		"0.20000": 20000,
		"1.00000": 1e5,
		"1.50000": 1e5 + 1e5/2,
		"1":       1e5,
		"2":       2e5,
		// 10 trillion, which is what's in circulation
		"10000000000000": 1e13 * 1e5,
		// Encountered an imprecise error
		"9974.99900": 997499900,
	}
	for in, expected := range validCases {
		actual, err := StrToQuarks(in)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)

		if strings.Contains(in, ".") {
			assert.Equal(t, in, StrFromQuarks(expected))
		} else {
			assert.Equal(t, fmt.Sprintf("%s.00000", in), StrFromQuarks(expected))
		}
	}
	// Ensure odd padding works.
	validCases = map[string]int64{
		"0.00001":      1,
		"0.00002":      2,
		"0.0002":       20,
		"0.002":        200,
		"0.02":         2000,
		"0.2":          20000,
		"1.000":        1e5,
		"1.500":        1e5 + 1e5/2,
		"1":            1e5,
		"2":            2e5,
		"341856.59000": 34185659000,
		"341856.59":    34185659000,
		"9974.99900":   997499900,
	}
	for in, expected := range validCases {
		actual, err := StrToQuarks(in)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	}

	invalidCases := []string{
		"0.000001",
		"0.000015",
		// 1000 trillion-1, ~100x more than what's in circulation
		"999999999999999",
		"abc",
		"10.-1",
		"10.0.0",
		".0",
	}
	for _, in := range invalidCases {
		actual, err := StrToQuarks(in)
		assert.Error(t, err)
		assert.Equal(t, int64(0), actual)
	}
}
