package thirdparty

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestGetAsciiBaseDomain(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
		isError  bool
	}{
		{
			input:    "app.getcode.com",
			expected: "getcode.com",
		},
		{
			input:    "app.getcode.com",
			expected: "getcode.com",
		},
		{
			input:    "subdomain.bücher.com",
			expected: "xn--bcher-kva.com",
		},
		{
			input:    "bücher.com",
			expected: "xn--bcher-kva.com",
		},
		{
			input:    "💩.domain.com",
			expected: "domain.com",
		},
		{
			input:    "google-énçøded.com",
			expected: "xn--google-nded-t9ay6s.com",
		},
		{
			input:    "subdomain.💩.io",
			expected: "xn--ls8h.io",
		},
		{
			input:   "localhost",
			isError: true,
		},
		{
			input:   fmt.Sprintf("%s.com", strings.Repeat("1", 64)),
			isError: true,
		},
		{
			input:   fmt.Sprintf("%s.com", strings.Repeat("1", 250)),
			isError: true,
		},
	} {
		actual, err := GetAsciiBaseDomain(tc.input)
		if tc.isError {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		}
	}
}

func TestGetDomainDisplayValue(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
		isError  bool
	}{
		{
			input:    "app.getcode.com",
			expected: "App.getcode.com",
		},
		{
			input:    "getcode.com",
			expected: "Getcode.com",
		},
		{
			input:    "UPPERCASE.com",
			expected: "Uppercase.com",
		},
		{
			input:    "xn--bcher-kva.com",
			expected: "Bücher.com",
		},
		{
			input:    "xn--cher-zra.com",
			expected: "Ücher.com",
		},
		{
			input:    "xn--google-nded-t9ay6s.com",
			expected: "Google-Énçøded.com",
		},
		{
			input:    "xn--ls8h.io",
			expected: "💩.io",
		},
		{
			input:   "localhost",
			isError: true,
		},
	} {
		actual, err := GetDomainDisplayName(tc.input)
		if tc.isError {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		}
	}
}

func TestVerifyDomainNameOwnership(t *testing.T) {
	ctx := context.Background()

	validAuthority, err := common.NewAccountFromPublicKeyString("7W6XCJmwXmVmzfkzSFUZ41cPtyvR35dPKF2sctLLEFDr")
	require.NoError(t, err)

	maliciousAuthority := testutil.NewRandomAccount(t)

	for _, authority := range []*common.Account{
		validAuthority,
		maliciousAuthority,
	} {
		isVerified, err := VerifyDomainNameOwnership(ctx, authority, "app.getcode.com")
		require.NoError(t, err)
		assert.Equal(t, authority.PublicKey().ToBase58() == validAuthority.PublicKey().ToBase58(), isVerified)
	}
}
