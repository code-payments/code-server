package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

func RunTests(t *testing.T, s webhook.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s webhook.Store){
		testHappyPath,
		testCounting,
		testWorkerQueries,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s webhook.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		time.Sleep(time.Millisecond)

		record := &webhook.Record{
			WebhookId: "webhook_id",
			Url:       "post.to.me/at/this/path:1234",
			Type:      webhook.TypeIntentSubmitted,

			Attempts: 0,
			State:    webhook.StateUnknown,

			NextAttemptAt: nil,
		}
		cloned := record.Clone()

		_, err := s.Get(ctx, record.WebhookId)
		assert.Equal(t, webhook.ErrNotFound, err)
		assert.Equal(t, webhook.ErrNotFound, s.Update(ctx, record))

		require.NoError(t, s.Put(ctx, record))
		assert.Equal(t, webhook.ErrAlreadyExists, s.Put(ctx, record))

		actual, err := s.Get(ctx, record.WebhookId)
		require.NoError(t, err)
		assert.True(t, actual.Id > 0)
		assert.True(t, actual.CreatedAt.After(start))
		assertEquivalentRecords(t, &cloned, actual)

		record.Attempts = 123
		record.State = webhook.StatePending
		record.NextAttemptAt = pointer.Time(time.Now().Add(5 * time.Second))
		cloned = record.Clone()
		require.NoError(t, s.Update(ctx, record))

		actual, err = s.Get(ctx, record.WebhookId)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testCounting(t *testing.T, s webhook.Store) {
	t.Run("testCounting", func(t *testing.T) {
		ctx := context.Background()

		records := []*webhook.Record{
			{WebhookId: "id1", Url: "url1", Type: webhook.TypeIntentSubmitted, Attempts: 0, State: webhook.StateUnknown, NextAttemptAt: nil},
			{WebhookId: "id2", Url: "url2", Type: webhook.TypeIntentSubmitted, Attempts: 1, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now())},
			{WebhookId: "id3", Url: "url3", Type: webhook.TypeIntentSubmitted, Attempts: 2, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now())},
			{WebhookId: "id4", Url: "url4", Type: webhook.TypeIntentSubmitted, Attempts: 3, State: webhook.StateConfirmed, NextAttemptAt: nil},
			{WebhookId: "id5", Url: "url5", Type: webhook.TypeIntentSubmitted, Attempts: 4, State: webhook.StateConfirmed, NextAttemptAt: nil},
			{WebhookId: "id6", Url: "url6", Type: webhook.TypeIntentSubmitted, Attempts: 5, State: webhook.StateConfirmed, NextAttemptAt: nil},
		}
		for _, record := range records {
			require.NoError(t, s.Put(ctx, record))
		}

		count, err := s.CountByState(ctx, webhook.StateUnknown)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		count, err = s.CountByState(ctx, webhook.StatePending)
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		count, err = s.CountByState(ctx, webhook.StateConfirmed)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		count, err = s.CountByState(ctx, webhook.StateFailed)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func testWorkerQueries(t *testing.T, s webhook.Store) {
	t.Run("testWorkerQueries", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllPendingReadyToSend(ctx, 10)
		assert.Equal(t, webhook.ErrNotFound, err)

		records := []*webhook.Record{
			{WebhookId: "id1", Url: "url1", Type: webhook.TypeIntentSubmitted, Attempts: 0, State: webhook.StateUnknown, NextAttemptAt: nil},
			{WebhookId: "id2", Url: "url2", Type: webhook.TypeIntentSubmitted, Attempts: 1, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now().Add(-2 * time.Second))},
			{WebhookId: "id3", Url: "url3", Type: webhook.TypeIntentSubmitted, Attempts: 2, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now().Add(-1 * time.Second))},
			{WebhookId: "id4", Url: "url4", Type: webhook.TypeIntentSubmitted, Attempts: 3, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now())},
			{WebhookId: "id5", Url: "url5", Type: webhook.TypeIntentSubmitted, Attempts: 4, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now().Add(time.Second))},
			{WebhookId: "id6", Url: "url6", Type: webhook.TypeIntentSubmitted, Attempts: 5, State: webhook.StatePending, NextAttemptAt: pointer.Time(time.Now().Add(2 * time.Second))},
			{WebhookId: "id7", Url: "url7", Type: webhook.TypeIntentSubmitted, Attempts: 6, State: webhook.StateConfirmed, NextAttemptAt: nil},
			{WebhookId: "id8", Url: "url8", Type: webhook.TypeIntentSubmitted, Attempts: 7, State: webhook.StateFailed, NextAttemptAt: nil},
		}
		for _, record := range records {
			require.NoError(t, s.Put(ctx, record))
		}

		actual, err := s.GetAllPendingReadyToSend(ctx, 10)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assertEquivalentRecords(t, records[1], actual[0])
		assertEquivalentRecords(t, records[2], actual[1])
		assertEquivalentRecords(t, records[3], actual[2])

		actual, err = s.GetAllPendingReadyToSend(ctx, 2)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[1], actual[0])
		assertEquivalentRecords(t, records[2], actual[1])
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *webhook.Record) {
	assert.Equal(t, obj1.WebhookId, obj2.WebhookId)
	assert.Equal(t, obj1.Url, obj2.Url)
	assert.Equal(t, obj1.Type, obj2.Type)
	assert.Equal(t, obj1.Attempts, obj2.Attempts)
	assert.Equal(t, obj1.State, obj2.State)

	if obj1.NextAttemptAt == nil {
		assert.Nil(t, obj2.NextAttemptAt)
	} else {
		require.NotNil(t, obj2.NextAttemptAt)
		assert.Equal(t, obj1.NextAttemptAt.Unix(), obj2.NextAttemptAt.Unix())
	}
}
