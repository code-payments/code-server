package transaction

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestNonce_SelectAvailableNonce(t *testing.T) {
	env := setupNonceTestEnv(t)

	allNonces := generateAvailableNonces(t, env, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction, 10)
	noncesByAddress := make(map[string]*nonce.Record)
	for _, nonceRecord := range allNonces {
		noncesByAddress[nonceRecord.Address] = nonceRecord
	}

	_, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeInternalServerProcess)
	assert.Equal(t, ErrNoAvailableNonces, err)

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentCvm, testutil.NewRandomAccount(t).PublicKey().ToBase58(), nonce.PurposeClientTransaction)
	assert.Equal(t, ErrNoAvailableNonces, err)

	selectedNonces := make(map[string]struct{})
	for i := 0; i < len(noncesByAddress); i++ {
		selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
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

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
	assert.Equal(t, ErrNoAvailableNonces, err)
}

func TestNonce_SelectAvailableNonceClaimed(t *testing.T) {
	env := setupNonceTestEnv(t)

	expiredNonces := map[string]*nonce.Record{}
	for i := 0; i < 10; i++ {
		n := generateClaimedNonce(t, env, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, true)
		expiredNonces[n.Address] = n

		generateClaimedNonce(t, env, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, false)
	}

	_, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeInternalServerProcess)
	assert.Equal(t, ErrNoAvailableNonces, err)

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentCvm, testutil.NewRandomAccount(t).PublicKey().ToBase58(), nonce.PurposeClientTransaction)
	assert.Equal(t, ErrNoAvailableNonces, err)

	for i := 0; i < 10; i++ {
		selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
		require.NoError(t, err)

		nonceRecord, ok := expiredNonces[selectedNonce.Account.PublicKey().ToBase58()]
		require.True(t, ok)
		require.True(t, nonceRecord.ClaimExpiresAt.Before(time.Now()))
		assert.Equal(t, nonceRecord.Address, selectedNonce.record.Address)
		assert.Equal(t, nonceRecord.Address, selectedNonce.Account.PublicKey().ToBase58())
		assert.Equal(t, nonceRecord.Blockhash, base58.Encode(selectedNonce.Blockhash[:]))
		delete(expiredNonces, selectedNonce.Account.PublicKey().ToBase58())

		updatedRecord, err := env.data.GetNonce(env.ctx, selectedNonce.Account.PublicKey().ToBase58())
		require.NoError(t, err)
		assert.Equal(t, nonce.StateReserved, updatedRecord.State)
		assert.Empty(t, updatedRecord.Signature)
	}

	_, err = SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
	require.Equal(t, ErrNoAvailableNonces, err)
}

func TestNonce_MarkReservedWithSignature(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonce(t, env, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
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

func TestNonce_ReleaseIfNotReserved(t *testing.T) {
	env := setupNonceTestEnv(t)

	generateAvailableNonce(t, env, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)

	selectedNonce, err := SelectAvailableNonce(env.ctx, env.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
	require.NoError(t, err)

	require.NoError(t, selectedNonce.ReleaseIfNotReserved())

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

func generateAvailableNonce(t *testing.T, env nonceTestEnv, nonceEnv nonce.Environment, instance string, useCase nonce.Purpose) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	rand.Read(bh[:])

	nonceKey := &vault.Record{
		PublicKey:  nonceAccount.PublicKey().ToBase58(),
		PrivateKey: nonceAccount.PrivateKey().ToBase58(),
		State:      vault.StateReserved,
		CreatedAt:  time.Now(),
	}
	nonceRecord := &nonce.Record{
		Address:             nonceAccount.PublicKey().ToBase58(),
		Authority:           common.GetSubsidizer().PublicKey().ToBase58(),
		Blockhash:           base58.Encode(bh[:]),
		Environment:         nonceEnv,
		EnvironmentInstance: instance,
		Purpose:             nonce.PurposeClientTransaction,
		State:               nonce.StateAvailable,
	}
	require.NoError(t, env.data.SaveKey(env.ctx, nonceKey))
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))
	return nonceRecord
}

func generateClaimedNonce(t *testing.T, env nonceTestEnv, nonceEnv nonce.Environment, instance string, expired bool) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	rand.Read(bh[:])

	nonceKey := &vault.Record{
		PublicKey:  nonceAccount.PublicKey().ToBase58(),
		PrivateKey: nonceAccount.PrivateKey().ToBase58(),
		State:      vault.StateReserved,
		CreatedAt:  time.Now(),
	}
	nonceRecord := &nonce.Record{
		Address:             nonceAccount.PublicKey().ToBase58(),
		Authority:           common.GetSubsidizer().PublicKey().ToBase58(),
		Blockhash:           base58.Encode(bh[:]),
		Environment:         nonceEnv,
		EnvironmentInstance: instance,
		Purpose:             nonce.PurposeClientTransaction,
		State:               nonce.StateClaimed,
		ClaimNodeID:         pointer.String("my_node_id"),
	}
	if expired {
		nonceRecord.ClaimExpiresAt = pointer.Time(time.Now().Add(-time.Hour))
	} else {
		nonceRecord.ClaimExpiresAt = pointer.Time(time.Now().Add(time.Hour))
	}

	require.NoError(t, env.data.SaveKey(env.ctx, nonceKey))
	require.NoError(t, env.data.SaveNonce(env.ctx, nonceRecord))
	return nonceRecord
}

func generateAvailableNonces(t *testing.T, env nonceTestEnv, nonceEnv nonce.Environment, instance string, useCase nonce.Purpose, count int) []*nonce.Record {
	var nonces []*nonce.Record
	for i := 0; i < count; i++ {
		nonces = append(nonces, generateAvailableNonce(t, env, nonceEnv, instance, useCase))
	}
	return nonces
}
