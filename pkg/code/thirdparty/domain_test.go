package thirdparty

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestVerifyDomainNameOwnership(t *testing.T) {
	ctx := context.Background()

	validAuthority, err := common.NewAccountFromPublicKeyString("McS32C1q6Rv1odkEoR5g1xtFBN7TdbkLFvGeyvQtzLF")
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
