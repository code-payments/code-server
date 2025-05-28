package transaction

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestLocalNoncePool(t *testing.T) {
	for _, tc := range []struct {
		name string
		tf   func(*localNoncePoolTest)
	}{
		{"HappyPath", testLocalNoncePoolHappyPath},
		{"Reload", testLocalNoncePoolReload},
		{"Refresh", testLocalNoncePoolRefresh},
		{"ReserveWithSignatureEdgeCases", testLocalNoncePoolReserveWithSignatureEdgeCases},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				nt := newLocalNoncePoolTest(t)
				defer nt.pool.Close()
				tc.tf(nt)
			},
		)
	}
}

func testLocalNoncePoolHappyPath(nt *localNoncePoolTest) {
	start := time.Now()

	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeOnDemandTransaction)
	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaDevnet, nonce.PurposeClientTransaction)
	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentCvm, testutil.NewRandomAccount(nt.t).PublicKey().ToBase58(), nonce.PurposeClientTransaction)

	ctx := context.Background()

	// None of the refresh periods should have kicked in, so our pool is empty.
	n, err := nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoAvailableNonces)
	require.Nil(nt.t, n)

	// Manually trigger a load
	_, err = nt.pool.load(ctx, nt.pool.opts.desiredPoolSize)
	require.NoError(nt.t, err)

	// Nonces should now be claimed and available in the pool
	observed := map[string]*Nonce{}
	for i := 0; i < nt.pool.opts.desiredPoolSize; i++ {
		n, err = nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		observed[n.record.Address] = n

		actual, err := nt.data.GetNonce(ctx, n.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.Environment, nonce.EnvironmentSolana)
		require.Equal(nt.t, actual.EnvironmentInstance, nonce.EnvironmentInstanceSolanaMainnet)
		require.Equal(nt.t, actual.Purpose, nonce.PurposeClientTransaction)
		require.Equal(nt.t, actual.State, nonce.StateClaimed)
		require.Equal(nt.t, *actual.ClaimNodeID, nt.pool.opts.nodeID)
		require.True(nt.t, actual.ClaimExpiresAt.After(start.Add(nt.pool.opts.minExpiration)))
		require.True(nt.t, actual.ClaimExpiresAt.Before(start.Add(nt.pool.opts.maxExpiration).Add(100*time.Millisecond)))
	}

	// Underlying local pool is empty
	n, err = nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoAvailableNonces)
	require.Nil(nt.t, n)

	// Reserve some nonces with a signature
	reserved := map[string]*Nonce{}
	for k, v := range observed {
		reserved[k] = v
		signature := fmt.Sprintf("signature%d", len(reserved))
		require.NoError(nt.t, v.MarkReservedWithSignature(ctx, signature))

		actual, err := nt.data.GetNonce(ctx, v.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateReserved)
		require.Equal(nt.t, actual.Signature, signature)
		require.Nil(nt.t, actual.ClaimNodeID)
		require.Nil(nt.t, actual.ClaimExpiresAt)

		if len(reserved) > len(observed)/2 {
			break
		}
	}

	// Releasing back to the pool should allow us to re-use the nonces that
	// weren't reserved
	for _, v := range observed {
		v.ReleaseIfNotReserved()
	}
	clear(observed)

	for i := 0; i < nt.pool.opts.desiredPoolSize-len(reserved); i++ {
		n, err = nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		require.NotContains(nt.t, reserved, n.record.Address)
		observed[n.record.Address] = n
	}

	// Underlying local pool is empty
	n, err = nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoAvailableNonces)
	require.Nil(nt.t, n)

	// Releasing back to the pool will allow us to make nonces available to
	// claim
	for _, v := range observed {
		v.ReleaseIfNotReserved()
	}

	require.NoError(nt.t, nt.pool.Close())
	n, err = nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoncePoolClosed)
	require.Nil(nt.t, n)

	// Reserved nonces should stay reserved
	for _, v := range reserved {
		actual, err := nt.data.GetNonce(ctx, v.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateReserved)
	}
	// Claimed nonces should become available to claim by another pool
	for _, v := range observed {
		actual, err := nt.data.GetNonce(ctx, v.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateAvailable)
	}
}

func testLocalNoncePoolReload(nt *localNoncePoolTest) {
	ctx := context.Background()

	nt.initializeNonces(10*nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)

	_, err := nt.pool.load(ctx, nt.pool.opts.desiredPoolSize)
	require.NoError(nt.t, err)

	observed := map[string]*Nonce{}
	for i := 0; i < 10*nt.pool.opts.desiredPoolSize; i++ {
		n, err := nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		observed[n.record.Address] = n

		actual, err := nt.data.GetNonce(ctx, n.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateClaimed)
		require.Equal(nt.t, *actual.ClaimNodeID, nt.pool.opts.nodeID)
		require.True(nt.t, actual.ClaimExpiresAt.After(time.Now()))

		// Allow nonce pool to reload
		if i%nt.pool.opts.desiredPoolSize == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func testLocalNoncePoolRefresh(nt *localNoncePoolTest) {
	ctx := context.Background()

	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)

	_, err := nt.pool.load(ctx, nt.pool.opts.desiredPoolSize)
	require.NoError(nt.t, err)

	// Wait out the max expiration, so every nonce's claim needs to be refreshed
	// at least once
	//
	// todo: Improve test running time
	time.Sleep(2 * nt.pool.opts.maxExpiration)

	// All nonces should still be claimed and available for use
	observed := map[string]*Nonce{}
	for i := 0; i < nt.pool.opts.desiredPoolSize; i++ {
		n, err := nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		observed[n.record.Address] = n

		actual, err := nt.data.GetNonce(ctx, n.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateClaimed)
		require.Equal(nt.t, *actual.ClaimNodeID, nt.pool.opts.nodeID)
		require.True(nt.t, actual.ClaimExpiresAt.After(time.Now()))
	}
}

func testLocalNoncePoolReserveWithSignatureEdgeCases(nt *localNoncePoolTest) {
	ctx := context.Background()

	nt.initializeNonces(nt.pool.opts.desiredPoolSize, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)

	_, err := nt.pool.load(ctx, nt.pool.opts.desiredPoolSize)
	require.NoError(nt.t, err)

	n, err := nt.pool.GetNonce(ctx)
	require.NoError(nt.t, err)
	require.NotNil(nt.t, n)

	require.Error(nt.t, n.MarkReservedWithSignature(ctx, ""))
	require.NoError(nt.t, n.MarkReservedWithSignature(ctx, "signature1"))
	require.NoError(nt.t, n.MarkReservedWithSignature(ctx, "signature1"))
	require.Error(nt.t, n.MarkReservedWithSignature(ctx, "signature2"))

	actual, err := nt.data.GetNonce(ctx, n.record.Address)
	require.NoError(nt.t, err)
	require.Equal(nt.t, actual.State, nonce.StateReserved)
	require.Equal(nt.t, actual.Signature, "signature1")
}

type localNoncePoolTest struct {
	t    *testing.T
	pool *LocalNoncePool
	data code_data.DatabaseData
}

func newLocalNoncePoolTest(t *testing.T) *localNoncePoolTest {
	data := code_data.NewTestDataProvider()

	pool, err := NewLocalNoncePool(
		data,
		nil,
		nonce.EnvironmentSolana,
		nonce.EnvironmentInstanceSolanaMainnet,
		nonce.PurposeClientTransaction,
		WithNoncePoolRefreshInterval(time.Second),
		WithNoncePoolRefreshPoolInterval(2*time.Second),
		WithNoncePoolMinExpiration(10*time.Second),
		WithNoncePoolMaxExpiration(15*time.Second),
	)
	require.NoError(t, err)

	return &localNoncePoolTest{
		t:    t,
		data: data,
		pool: pool,
	}
}

func (np *localNoncePoolTest) initializeNonces(amount int, env nonce.Environment, envInstance string, purpose nonce.Purpose) {
	for i := 0; i < amount; i++ {
		err := np.data.SaveNonce(context.Background(), &nonce.Record{
			Address:             fmt.Sprintf("addr-%s-%s-%s-%d", env.String(), envInstance, purpose.String(), i),
			Authority:           "my authority!",
			Environment:         env,
			EnvironmentInstance: envInstance,
			Purpose:             purpose,
			State:               nonce.StateAvailable,
		})
		require.NoError(np.t, err)
	}
}
