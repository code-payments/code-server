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
		testExpiredRecord,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s rendezvous.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()

		record := &rendezvous.Record{
			Key:       "key",
			Address:   "localhost:1234",
			ExpiresAt: time.Now().Add(time.Second),
		}
		cloned := record.Clone()

		require.NoError(t, s.Delete(ctx, record.Key, record.Address))
		_, err := s.Get(ctx, record.Key)
		assert.Equal(t, rendezvous.ErrNotFound, err)
		assert.Equal(t, rendezvous.ErrNotFound, s.ExtendExpiry(ctx, record.Key, record.Address, time.Now().Add(time.Minute)))

		require.NoError(t, s.Put(ctx, record))

		actual, err := s.Get(ctx, record.Key)
		require.NoError(t, err)
		assert.True(t, actual.Id > 0)
		assert.True(t, actual.CreatedAt.After(start))
		assertEquivalentRecords(t, &cloned, actual)

		time.Sleep(time.Millisecond)
		record.Address = "localhost:5678"
		record.ExpiresAt = time.Now().Add(2 * time.Second)
		cloned = record.Clone()
		require.Equal(t, rendezvous.ErrExists, s.Put(ctx, record))

		time.Sleep(time.Second)
		require.NoError(t, s.Put(ctx, record))

		actual, err = s.Get(ctx, record.Key)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		require.NoError(t, s.Delete(ctx, record.Key, "other:8888"))

		actual, err = s.Get(ctx, record.Key)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		require.NoError(t, s.Delete(ctx, record.Key, record.Address))

		_, err = s.Get(ctx, record.Key)
		assert.Equal(t, rendezvous.ErrNotFound, err)
	})
}

func testExpiredRecord(t *testing.T, s rendezvous.Store) {
	t.Run("testExpiredRecord", func(t *testing.T) {
		ctx := context.Background()

		record := &rendezvous.Record{
			Key:       "key",
			Address:   "localhost:1234",
			ExpiresAt: time.Now().Add(100 * time.Millisecond),
		}
		require.NoError(t, s.Put(ctx, record))

		time.Sleep(200 * time.Millisecond)

		_, err := s.Get(ctx, record.Key)
		assert.Equal(t, rendezvous.ErrNotFound, err)
		assert.Equal(t, rendezvous.ErrNotFound, s.ExtendExpiry(ctx, record.Key, record.Address, time.Now().Add(time.Minute)))

		require.NoError(t, s.Delete(ctx, record.Key, record.Address))
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *rendezvous.Record) {
	assert.Equal(t, obj1.Key, obj2.Key)
	assert.Equal(t, obj1.Address, obj2.Address)
	assert.Equal(t, obj1.ExpiresAt.Unix(), obj2.ExpiresAt.Unix())
}
