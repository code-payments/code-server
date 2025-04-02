package async_webhook

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/server/messaging"
	webhook_util "github.com/code-payments/code-server/pkg/code/webhook"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestWorker_HappyPath(t *testing.T) {
	env := setup(t)

	record := env.webhook.GetRandomWebhookRecord(t, webhook.TypeTest)
	require.NoError(t, env.data.CreateWebhook(env.ctx, record))
	env.handlePending(t, record, false)

	assert.Len(t, env.webhook.GetReceivedRequests(), 1)
	env.assertWebhookState(t, record.WebhookId, webhook.StateConfirmed, 1)
}

func TestWorker_FailurePath(t *testing.T) {
	env := setup(t)

	for prevAttempt := uint8(0); int(prevAttempt) < len(attemptToDelay); prevAttempt++ {
		env.webhook.SimulateErrors()

		record := env.webhook.GetRandomWebhookRecord(t, webhook.TypeTest)
		record.Attempts = prevAttempt
		require.NoError(t, env.data.CreateWebhook(env.ctx, record))

		if int(prevAttempt)+1 == len(attemptToDelay) {
			env.handlePending(t, record, false)
			assert.Empty(t, env.webhook.GetReceivedRequests())
		} else {
			env.handlePending(t, record, true)
			assert.Len(t, env.webhook.GetReceivedRequests(), 1)
		}

		expectedState := webhook.StatePending
		expectedAttemptCount := prevAttempt + 1
		if int(prevAttempt)+1 == len(attemptToDelay) {
			expectedState = webhook.StateFailed
			expectedAttemptCount = prevAttempt
		}
		env.assertWebhookState(t, record.WebhookId, expectedState, expectedAttemptCount)

		env.webhook.Reset()
	}
}

func TestWorker_ConsistentStateManagement(t *testing.T) {
	env := setup(t)

	// Confirmation with next attempt setup in a later webhook execution

	for attempt := uint8(1); int(attempt) < len(attemptToDelay); attempt++ {
		record := env.webhook.GetRandomWebhookRecord(t, webhook.TypeTest)
		require.NoError(t, env.data.CreateWebhook(env.ctx, record))

		cloned := record.Clone()
		cloned.Attempts = attempt
		cloned.State = webhook.StatePending
		require.NoError(t, env.data.UpdateWebhook(env.ctx, &cloned))

		env.handlePending(t, record, false)

		assert.Len(t, env.webhook.GetReceivedRequests(), 1)
		env.assertWebhookState(t, record.WebhookId, webhook.StateConfirmed, 1)

		env.webhook.Reset()
	}

	// Already confirmed

	env.webhook.Reset()
	env.webhook.SimulateErrors()

	record := env.webhook.GetRandomWebhookRecord(t, webhook.TypeTest)
	record.Attempts = uint8(len(attemptToDelay))
	require.NoError(t, env.data.CreateWebhook(env.ctx, record))

	cloned := record.Clone()
	cloned.Attempts = 1
	cloned.NextAttemptAt = nil
	cloned.State = webhook.StateConfirmed
	require.NoError(t, env.data.UpdateWebhook(env.ctx, &cloned))

	env.handlePending(t, record, false)

	assert.Empty(t, env.webhook.GetReceivedRequests())
	env.assertWebhookState(t, record.WebhookId, webhook.StateConfirmed, 1)

	// Failed, but later confirmed

	env.webhook.Reset()

	record = env.webhook.GetRandomWebhookRecord(t, webhook.TypeTest)
	require.NoError(t, env.data.CreateWebhook(env.ctx, record))

	cloned = record.Clone()
	cloned.Attempts = uint8(len(attemptToDelay)) - 1
	cloned.NextAttemptAt = nil
	cloned.State = webhook.StateFailed
	require.NoError(t, env.data.UpdateWebhook(env.ctx, &cloned))

	env.handlePending(t, record, false)

	assert.Len(t, env.webhook.GetReceivedRequests(), 1)
	env.assertWebhookState(t, record.WebhookId, webhook.StateConfirmed, 1)
}

type testEnv struct {
	ctx     context.Context
	data    code_data.Provider
	worker  *service
	webhook *webhook_util.TestWebhookEndpoint
}

func setup(t *testing.T) *testEnv {
	data := code_data.NewTestDataProvider()
	testutil.SetupRandomSubsidizer(t, data)
	return &testEnv{
		ctx:  context.Background(),
		data: data,
		worker: New(
			data,
			messaging.NewMessagingClient(data),
			withManualTestOverrides(&testOverrides{}),
		).(*service),
		webhook: webhook_util.NewTestWebhookEndpoint(t),
	}
}

func (e *testEnv) handlePending(t *testing.T, record *webhook.Record, shouldError bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	err := e.worker.handlePending(e.ctx, record, &wg)
	if shouldError {
		assert.Error(t, err)
	} else {
		require.NoError(t, err)
	}
}

func (e *testEnv) assertWebhookState(t *testing.T, id string, expectedState webhook.State, expectedAttempts uint8) {
	record, err := e.data.GetWebhook(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, expectedState, record.State)
	assert.Equal(t, expectedAttempts, record.Attempts)

	if record.State == webhook.StatePending {
		require.NotNil(t, record.NextAttemptAt)
		timeUntilNextAttempt := time.Until(*record.NextAttemptAt)
		assert.True(t, timeUntilNextAttempt <= attemptToDelay[expectedAttempts+1])
		assert.True(t, timeUntilNextAttempt > attemptToDelay[expectedAttempts+1]-50*time.Millisecond)
	} else {
		assert.Nil(t, record.NextAttemptAt)
	}
}
