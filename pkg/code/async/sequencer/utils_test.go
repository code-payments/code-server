package async_sequencer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
)

func TestMarkNonceAsAvailableDueToRevokedFulfillment_SafetyChecks(t *testing.T) {
	env := setupWorkerEnv(t)

	fulfillmentRecord := env.createAnyFulfillmentInState(t, fulfillment.StateUnknown)

	nonceRecord, err := env.data.GetNonce(env.ctx, *fulfillmentRecord.Nonce)
	require.NoError(t, err)

	nonceRecord.State = nonce.StateAvailable
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	err = env.worker.markNonceAvailableDueToRevokedFulfillment(env.ctx, fulfillmentRecord)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unexpected nonce state"))

	nonceRecord.State = nonce.StateReleased
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	err = env.worker.markNonceAvailableDueToRevokedFulfillment(env.ctx, fulfillmentRecord)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unexpected nonce state"))

	nonceRecord.Blockhash = "blockhash"
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	err = env.worker.markNonceAvailableDueToRevokedFulfillment(env.ctx, fulfillmentRecord)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unexpected nonce blockhash"))

	nonceRecord.Signature = "signature"
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	err = env.worker.markNonceAvailableDueToRevokedFulfillment(env.ctx, fulfillmentRecord)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unexpected nonce signature"))

	fulfillmentRecord.State = fulfillment.StatePending
	err = env.worker.markNonceAvailableDueToRevokedFulfillment(env.ctx, fulfillmentRecord)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "fulfillment is in dangerous state to manage nonce state"))

	actualNonceRecord, err := env.data.GetNonce(env.ctx, *fulfillmentRecord.Nonce)
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReleased, actualNonceRecord.State)
	assert.Equal(t, "signature", actualNonceRecord.Signature)
	assert.Equal(t, "blockhash", actualNonceRecord.Blockhash)
}
