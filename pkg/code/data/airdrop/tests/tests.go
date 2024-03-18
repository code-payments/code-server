package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/airdrop"
)

func RunTests(t *testing.T, s airdrop.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s airdrop.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s airdrop.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		isEligible, err := s.IsEligible(ctx, "owner")
		require.NoError(t, err)
		assert.True(t, isEligible)

		for i := 0; i < 3; i++ {
			require.NoError(t, s.MarkIneligible(ctx, "owner"))

			isEligible, err = s.IsEligible(ctx, "owner")
			require.NoError(t, err)
			assert.False(t, isEligible)
		}
	})
}
