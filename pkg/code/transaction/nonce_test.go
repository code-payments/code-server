package transaction

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/vault"
)

func TestNonce_SelectAvailableNonce(t *testing.T) {
	env := setupNonceTestEnv(t)

	allNonces := generateAvailableNonces(t, env, nonce.PurposeClientTransaction, 10)
	noncesByAddress := make(map[string]*nonce.Record)
	for _, nonceRecord := range allNonces {
		noncesByAddress[nonceRecord.Address] = nonceRecord
	}

	_, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeInternalServerProcess)
	assert.Equal(t, ErrNoAvailableNonces, err)

	selectedNonces := make(map[string]struct{})
	for i := 0; i < len(noncesByAddress); i++ {
		selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
		require.NoError(t, err)

		_, ok := selectedNonces[selectedNonce.Account.PublicKey().ToBase58()]
		assert.False(t, ok)

		nonceRecord, ok := noncesByAddress[selectedNonce.Account.PublicKey().ToBase58()]
		require.True(t, ok)
		assert.Equal(t, nonceRecord.Address, selectedNonce.record.Address)
		assert.Equal(t, nonceRecord.Address, selectedNonce.Account.PublicKey().ToBase58())
		assert.Equal(t, nonceRecord.Blockhash, base58.Encode(selectedNonce.Blockhash[:]))

		selectedNonces[selectedNonce.record.Address] = struct{}{}

		updatedRecord, err := env.data.GetNonce(env.ctx, nonceRecord.Address)
		require.NoError(t, err)
		assert.Equal(t, nonce.StateReserved, updatedRecord.State)
		assert.Empty(t, updatedRecord.Signature)
	}
	assert.Len(t, selectedNonces, len(noncesByAddress))

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	assert.Equal(t, ErrNoAvailableNonces, err)

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.PurposeInternalServerProcess)
	assert.Equal(t, ErrNoAvailableNonces, err)
}

func TestNonce_SelectNonceFromFulfillmentToUpgrade_HappyPath(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonces(t, env, nonce.PurposeClientTransaction, 2)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	require.NoError(t, err)

	fulfillmentToUpgrade := &fulfillment.Record{
		Nonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
		Blockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),
		Signature: pointer.String("signature"),
	}

	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, *fulfillmentToUpgrade.Signature))

	selectedNonce.Unlock()

	selectedNonce, err = SelectNonceFromFulfillmentToUpgrade(env.ctx, env.data, fulfillmentToUpgrade)
	require.NoError(t, err)

	assert.Equal(t, *fulfillmentToUpgrade.Nonce, selectedNonce.Account.PublicKey().ToBase58())
	assert.Equal(t, *fulfillmentToUpgrade.Blockhash, base58.Encode(selectedNonce.Blockhash[:]))

	require.NoError(t, selectedNonce.UpdateSignature(env.ctx, "new_signature"))

	updatedRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReserved, updatedRecord.State)
	assert.Equal(t, "new_signature", updatedRecord.Signature)

	selectedNonce.Unlock()

	_, err = SelectNonceFromFulfillmentToUpgrade(env.ctx, env.data, fulfillmentToUpgrade)
	assert.Error(t, err)
}

func TestNonce_SelectNonceFromFulfillmentToUpgrade_DangerousPath(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonces(t, env, nonce.PurposeClientTransaction, 2)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	require.NoError(t, err)

	fulfillmentToUpgrade := &fulfillment.Record{
		Nonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
		Blockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),
		Signature: pointer.String("signature"),
	}

	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, *fulfillmentToUpgrade.Signature))

	selectedNonce.Unlock()

	nonceRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)

	originalBlockhash := nonceRecord.Blockhash
	nonceRecord.Blockhash = "Cmui8pHYbKKox8g7n7xa2Qaxh1TSJsHpr3xCeNaEisdy"
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	_, err = SelectNonceFromFulfillmentToUpgrade(env.ctx, env.data, fulfillmentToUpgrade)
	assert.Error(t, err)

	nonceRecord.Blockhash = originalBlockhash
	nonceRecord.State = nonce.StateAvailable
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	_, err = SelectNonceFromFulfillmentToUpgrade(env.ctx, env.data, fulfillmentToUpgrade)
	assert.Error(t, err)

	nonceRecord.State = nonce.StateReserved
	nonceRecord.Signature = "other_signature"
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))

	_, err = SelectNonceFromFulfillmentToUpgrade(env.ctx, env.data, fulfillmentToUpgrade)
	assert.Error(t, err)
}

func TestNonce_MarkReservedWithSignature(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonce(t, env, nonce.PurposeClientTransaction)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	require.NoError(t, err)

	assert.Error(t, selectedNonce.MarkReservedWithSignature(env.ctx, ""))
	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, "signature1"))
	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, "signature1"))
	assert.Error(t, selectedNonce.MarkReservedWithSignature(env.ctx, "signature2"))

	updatedRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReserved, updatedRecord.State)
	assert.Equal(t, "signature1", updatedRecord.Signature)
}

func TestNonce_UpdateSignature(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonce(t, env, nonce.PurposeClientTransaction)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	require.NoError(t, err)
	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, "signature1"))

	assert.Error(t, selectedNonce.UpdateSignature(env.ctx, ""))
	require.NoError(t, selectedNonce.UpdateSignature(env.ctx, "signature1"))
	require.NoError(t, selectedNonce.UpdateSignature(env.ctx, "signature2"))

	updatedRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReserved, updatedRecord.State)
	assert.Equal(t, "signature2", updatedRecord.Signature)
}

func TestNonce_ReleaseIfNotReserved(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonce(t, env, nonce.PurposeClientTransaction)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.PurposeClientTransaction)
	require.NoError(t, err)

	require.NoError(t, selectedNonce.ReleaseIfNotReserved())

	assert.Error(t, selectedNonce.UpdateSignature(env.ctx, "signature"))

	updatedRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, nonce.StateAvailable, updatedRecord.State)
	assert.Empty(t, updatedRecord.Signature)

	require.NoError(t, selectedNonce.MarkReservedWithSignature(env.ctx, "signature"))
	require.NoError(t, selectedNonce.ReleaseIfNotReserved())

	updatedRecord, err = env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, nonce.StateReserved, updatedRecord.State)
	assert.Equal(t, "signature", updatedRecord.Signature)
}

type nonceTestEnv struct {
	ctx  context.Context
	data code_data.Provider
}

func setupNonceTestEnv(t *testing.T) nonceTestEnv {
	data := code_data.NewTestDataProvider()

	testutil.SetupRandomSubsidizer(t, data)

	return nonceTestEnv{
		ctx:  context.Background(),
		data: data,
	}
}

func generateAvailableNonce(t *testing.T, env nonceTestEnv, useCase nonce.Purpose) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	rand.Read(bh[:])

	nonceKey := &vault.Record{
		PublicKey:  nonceAccount.PublicKey().ToBase58(),
		PrivateKey: nonceAccount.PrivateKey().ToBase58(),
		State:      vault.StateAvailable,
		CreatedAt:  time.Now(),
	}
	nonceRecord := &nonce.Record{
		Address:   nonceAccount.PublicKey().ToBase58(),
		Authority: common.GetSubsidizer().PublicKey().ToBase58(),
		Blockhash: base58.Encode(bh[:]),
		Purpose:   nonce.PurposeClientTransaction,
		State:     nonce.StateAvailable,
	}
	require.NoError(t, env.data.SaveKey(env.ctx, nonceKey))
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))
	return nonceRecord
}

func generateAvailableNonces(t *testing.T, env nonceTestEnv, useCase nonce.Purpose, count int) []*nonce.Record {
	var nonces []*nonce.Record
	for i := 0; i < count; i++ {
		nonces = append(nonces, generateAvailableNonce(t, env, useCase))
	}
	return nonces
}
