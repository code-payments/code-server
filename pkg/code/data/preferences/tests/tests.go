package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

func RunTests(t *testing.T, s preferences.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s preferences.Store){
		testRoundTrip,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s preferences.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		containerId := user.NewDataContainerID()

		_, err := s.Get(ctx, containerId)
		assert.Equal(t, preferences.ErrPreferencesNotFound, err)

		expected := preferences.GetDefaultPreferences(containerId)
		cloned := expected.Clone()

		start := time.Now()
		time.Sleep(time.Millisecond)

		require.NoError(t, s.Save(ctx, expected))
		assert.True(t, expected.Id > 0)
		assert.True(t, expected.LastUpdatedAt.After(start))

		actual, err := s.Get(ctx, containerId)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		expected.Locale = language.CanadianFrench
		cloned = expected.Clone()

		start = time.Now()
		time.Sleep(time.Millisecond)

		require.NoError(t, s.Save(ctx, expected))
		assert.Equal(t, cloned.Id, expected.Id)
		assert.True(t, expected.LastUpdatedAt.After(start))

		actual, err = s.Get(ctx, containerId)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *preferences.Record) {
	assert.Equal(t, obj1.DataContainerId, obj2.DataContainerId)
	assert.Equal(t, obj1.Locale.String(), obj2.Locale.String())
}
