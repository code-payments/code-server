package transaction

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
)

func TestNoncePool(t *testing.T) {
	for _, tc := range []struct {
		name string
		tf   func(*noncePoolTest)
	}{
		{"Happy", testNoncePoolHappy},
		{"RefreshSignal", testNonceRefreshSignal},
		{"Refresh", testNoncePoolRefresh},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				nt := newNoncePoolTest(t)
				defer nt.pool.Close()
				tc.tf(nt)
			},
		)
	}
}

func testNoncePoolHappy(nt *noncePoolTest) {
	nt.initializeNonces(100, nonce.PurposeClientTransaction)
	nt.initializeNonces(100, nonce.PurposeOnDemandTransaction)

	ctx := context.Background()

	// Since our time hasn't advanced, none of the refresh periods
	// should have kicked in, and therefore our pool is empty.
	n, err := nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoAvailableNonces)
	require.Nil(nt.t, n)

	nt.clock.Advance(2 * time.Second)
	time.Sleep(100 * time.Millisecond)

	observed := map[string]*Nonce{}
	for i := 0; i < 100; i++ {
		n, err = nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		observed[n.record.Address] = n

		actual, err := nt.data.GetNonce(ctx, n.record.Address)
		require.NoError(nt.t, err)
		require.Equal(nt.t, actual.State, nonce.StateClaimed)
		require.Equal(nt.t, actual.ClaimNodeId, nt.pool.opts.nodeId)
	}

	// Underlying DB pool is empty.
	n, err = nt.pool.GetNonce(ctx)
	require.ErrorIs(nt.t, err, ErrNoAvailableNonces)
	require.Nil(nt.t, n)

	// Releasing back to the pool should allow us to
	// re-use the nonces.
	for _, v := range observed {
		v.ReleaseIfNotReserved()
	}
	clear(observed)

	for i := 0; i < 100; i++ {
		n, err = nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
		require.NotNil(nt.t, n)
		require.NotContains(nt.t, observed, n.record.Address)
		observed[n.record.Address] = n
	}
}

func testNonceRefreshSignal(nt *noncePoolTest) {
	nt.initializeNonces(100, nonce.PurposeClientTransaction)
	nt.initializeNonces(100, nonce.PurposeOnDemandTransaction)

	ctx := context.Background()

	require.NoError(nt.t, nt.pool.Load(ctx, 10))

	for i := 0; i < 10; i++ {
		_, err := nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
	}

	// Note: We're sleeping in real time to let the background
	// process run, but we haven't advanced the clock, so the
	// only way a refresh could occur is via the signal.
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		_, err := nt.pool.GetNonce(ctx)
		require.NoError(nt.t, err)
	}
}

func testNoncePoolRefresh(nt *noncePoolTest) {
	nt.initializeNonces(100, nonce.PurposeClientTransaction)
	nt.initializeNonces(100, nonce.PurposeOnDemandTransaction)

	ctx := context.Background()
	require.NoError(nt.t, nt.pool.Load(ctx, 10))

	nonceExpirations := map[string]time.Time{}

	nt.pool.freeListMu.Lock()
	for _, n := range nt.pool.freeList {
		nonceExpirations[n.record.Address] = n.record.ClaimExpiresAt
	}
	nt.pool.freeListMu.Unlock()

	nt.clock.Advance(3 * time.Minute)
	time.Sleep(100 * time.Millisecond)

	refreshedNonceExpirations := map[string]time.Time{}
	nt.pool.freeListMu.Lock()
	for _, n := range nt.pool.freeList {
		refreshedNonceExpirations[n.record.Address] = n.record.ClaimExpiresAt
	}
	nt.pool.freeListMu.Unlock()

	refreshed := 0
	for nonce, expiration := range refreshedNonceExpirations {
		if nonceExpirations[nonce].Before(expiration) {
			refreshed++
		}
	}

	require.Greater(nt.t, refreshed, 75)

	for n, expiration := range refreshedNonceExpirations {
		actual, err := nt.data.GetNonce(ctx, n)
		require.NoError(nt.t, err)

		require.Equal(nt.t, actual.State, nonce.StateClaimed)
		require.Equal(nt.t, actual.ClaimNodeId, nt.pool.opts.nodeId)
		require.True(nt.t, actual.ClaimExpiresAt.Equal(expiration))
	}
}

type noncePoolTest struct {
	t     *testing.T
	clock clockwork.FakeClock
	pool  *NoncePool
	data  code_data.DatabaseData
}

func newNoncePoolTest(t *testing.T) *noncePoolTest {
	clock := clockwork.NewFakeClock()
	data := code_data.NewTestDataProvider()

	pool, err := NewNoncePool(
		data,
		nonce.PurposeClientTransaction,
		WithNoncePoolClock(clock),
		WithNoncePoolRefreshInterval(time.Second),
		WithNoncePoolRefreshPoolInterval(2*time.Second),
	)
	require.NoError(t, err)

	return &noncePoolTest{
		t:     t,
		clock: clock,
		data:  data,
		pool:  pool,
	}
}

func (np *noncePoolTest) initializeNonces(amount int, purpose nonce.Purpose) {
	for i := 0; i < amount; i++ {
		err := np.data.SaveNonce(context.Background(), &nonce.Record{
			Id:        uint64(i) + 1,
			Address:   fmt.Sprintf("addr-%s-%d", purpose.String(), i),
			Authority: "my authority!",
			Purpose:   purpose,
			State:     nonce.StateAvailable,
		})
		require.NoError(np.t, err)
	}
}
