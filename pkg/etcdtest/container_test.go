package etcdtest

import (
	"context"
	"fmt"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdContainer(t *testing.T) {
	ctx := context.Background()
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	get, err := client.Get(ctx, "/", clientv3.WithPrefix())
	require.NoError(err)
	require.Empty(get.Kvs)

	for i := 0; i < 10; i++ {
		_, err := client.Put(ctx, fmt.Sprintf("/%d", i), fmt.Sprintf("value-%d", i))
		require.NoError(err)
	}

	get, err = client.Get(ctx, "/", clientv3.WithPrefix())
	require.NoError(err)
	require.Len(get.Kvs, 10)

	for i := 0; i < 10; i++ {
		require.Equal(fmt.Sprintf("/%d", i), string(get.Kvs[i].Key))
		require.Equal(fmt.Sprintf("value-%d", i), string(get.Kvs[i].Value))
		require.EqualValues(2+i, get.Kvs[i].CreateRevision)
	}
}
