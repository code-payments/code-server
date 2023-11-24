package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/badgecount"
)

func RunTests(t *testing.T, s badgecount.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s badgecount.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s badgecount.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()
		time.Sleep(time.Millisecond)

		_, err := s.Get(ctx, "owner1")
		assert.Equal(t, badgecount.ErrBadgeCountNotFound, err)

		require.NoError(t, s.Add(ctx, "owner1", 1))
		require.NoError(t, s.Add(ctx, "owner2", 5))

		actual, err := s.Get(ctx, "owner1")
		require.NoError(t, err)
		require.NoError(t, actual.Validate())
		assert.True(t, actual.Id > 0)
		assert.Equal(t, "owner1", actual.Owner)
		assert.EqualValues(t, 1, actual.BadgeCount)
		assert.True(t, actual.LastUpdatedAt.After(start))
		assert.True(t, actual.CreatedAt.After(start))

		start = time.Now()
		time.Sleep(time.Millisecond)

		require.NoError(t, s.Add(ctx, "owner1", 2))

		actual, err = s.Get(ctx, "owner1")
		require.NoError(t, err)
		require.NoError(t, actual.Validate())
		assert.Equal(t, "owner1", actual.Owner)
		assert.EqualValues(t, 3, actual.BadgeCount)
		assert.True(t, actual.LastUpdatedAt.After(start))
		assert.True(t, actual.CreatedAt.Before(start))

		start = time.Now()
		time.Sleep(time.Millisecond)

		require.NoError(t, s.Reset(ctx, "owner1"))

		actual, err = s.Get(ctx, "owner1")
		require.NoError(t, err)
		require.NoError(t, actual.Validate())
		assert.Equal(t, "owner1", actual.Owner)
		assert.EqualValues(t, 0, actual.BadgeCount)
		assert.True(t, actual.LastUpdatedAt.After(start))
		assert.True(t, actual.CreatedAt.Before(start))

		actual, err = s.Get(ctx, "owner2")
		require.NoError(t, err)
		require.NoError(t, actual.Validate())
		assert.Equal(t, "owner2", actual.Owner)
		assert.EqualValues(t, 5, actual.BadgeCount)
	})
}
