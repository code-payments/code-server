package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimated_Account(t *testing.T) {
	ctx := context.Background()

	provider, err := NewEstimatedProvider()
	require.NoError(t, err)

	err = provider.AddKnownAccount(ctx, []byte("account_1"))
	require.NoError(t, err)

	err = provider.AddKnownAccount(ctx, []byte("account_2"))
	require.NoError(t, err)

	found, err := provider.TestForKnownAccount(ctx, []byte("account_x"))
	require.NoError(t, err)
	assert.False(t, found)

	found, err = provider.TestForKnownAccount(ctx, []byte("account_1"))
	require.NoError(t, err)
	assert.True(t, found)

	found, err = provider.TestForKnownAccount(ctx, []byte("account_2"))
	require.NoError(t, err)
	assert.True(t, found)
}
