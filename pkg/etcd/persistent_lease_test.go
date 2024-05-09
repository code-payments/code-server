package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/code-payments/code-server/pkg/etcdtest"
)

func TestPersistentLease(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	key := "/cluster/me"
	value := "initial"

	pl, err := NewPersistentLease(client, key, value, 10*time.Second)
	require.NoError(err)

	// Ensure the KV is recreated on:
	//   1. External removal of KV.
	//   2. The lease is revoked.
	for i := 0; i < 5; i++ {
		require.Eventually(func() bool {
			resp, err := client.Get(context.Background(), key)
			require.NoError(err)
			return len(resp.Kvs) == 1
		}, 3*time.Second, 100*time.Millisecond)

		resp, err := client.Get(context.Background(), key)
		require.NoError(err)
		require.Len(resp.Kvs, 1)
		require.Equal(key, string(resp.Kvs[0].Key))
		require.Equal(value, string(resp.Kvs[0].Value))
		require.NotZero(resp.Kvs[0].Lease)

		if i%2 == 0 {
			_, err = client.Delete(context.Background(), key)
			require.NoError(err)
		} else {
			_, err = client.Revoke(context.Background(), v3.LeaseID(resp.Kvs[0].Lease))
			require.NoError(err)
		}
	}

	pl.SetValue("modified")
	require.Eventually(func() bool {
		resp, err := client.Get(context.Background(), key)
		if err != nil {
			return false
		}
		return len(resp.Kvs) == 1 && string(resp.Kvs[0].Value) == "modified"
	}, 3*time.Second, 100*time.Millisecond)

	pl.Close()

	require.Eventually(func() bool {
		resp, err := client.Get(context.Background(), key)
		if err != nil {
			return false
		}
		return len(resp.Kvs) == 0
	}, 3*time.Second, 100*time.Millisecond)
}
