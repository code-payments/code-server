package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
)

func RunTests(t *testing.T, s rendezvous.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s rendezvous.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s rendezvous.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		time.Sleep(time.Millisecond)

		record := &rendezvous.Record{
			Key:      "key",
			Location: "localhost:1234",
		}
		cloned := record.Clone()

		require.NoError(t, s.Delete(ctx, record.Key))
		_, err := s.Get(ctx, record.Key)
		assert.Equal(t, rendezvous.ErrNotFound, err)

		require.NoError(t, s.Save(ctx, record))

		actual, err := s.Get(ctx, record.Key)
		require.NoError(t, err)
		assert.True(t, actual.Id > 0)
		assert.True(t, actual.CreatedAt.After(start))
		assert.True(t, actual.LastUpdatedAt.After(start))
		assertEquivalentRecords(t, &cloned, actual)

		updateTime := time.Now()
		time.Sleep(time.Millisecond)
		record.Location = "localhost:5678"
		cloned = record.Clone()
		require.NoError(t, s.Save(ctx, record))

		actual, err = s.Get(ctx, record.Key)
		require.NoError(t, err)
		assert.True(t, actual.CreatedAt.Before(updateTime))
		assert.True(t, actual.LastUpdatedAt.After(updateTime))
		assertEquivalentRecords(t, &cloned, actual)

		require.NoError(t, s.Delete(ctx, record.Key))

		_, err = s.Get(ctx, record.Key)
		assert.Equal(t, rendezvous.ErrNotFound, err)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *rendezvous.Record) {
	assert.Equal(t, obj1.Key, obj2.Key)
	assert.Equal(t, obj1.Location, obj2.Location)
}
