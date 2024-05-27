package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/etcdtest"
)

func TestWaitCreate(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	resultCh := make(chan error)
	go func() {
		resultCh <- WaitFor(context.Background(), client, "/wait", true)
	}()

	select {
	case <-resultCh:
		require.Fail("should not have yielded")
	case <-time.After(time.Second):
	}

	_, err = client.Put(context.Background(), "/wait", "1")
	require.NoError(err)
	require.NoError(<-resultCh)

	require.NoError(WaitFor(context.Background(), client, "/wait", true))
}

func TestWaitDelete(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	require.NoError(WaitFor(context.Background(), client, "/wait", false))

	_, err = client.Put(context.Background(), "/wait", "1")
	require.NoError(err)

	resultCh := make(chan error)
	go func() {
		resultCh <- WaitFor(context.Background(), client, "/wait", false)
	}()

	select {
	case <-resultCh:
		require.Fail("should not have yielded")
	case <-time.After(time.Second):
	}

	_, err = client.Delete(context.Background(), "/wait")
	require.NoError(err)
	require.NoError(<-resultCh)
}

func TestWaitTimeout(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	err = WaitFor(ctx, client, "/wait", true)
	elapsed := time.Since(start)

	require.ErrorIs(err, context.DeadlineExceeded)
	require.GreaterOrEqual(elapsed, time.Second)
}
