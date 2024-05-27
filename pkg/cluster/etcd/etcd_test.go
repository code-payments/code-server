package etcd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/code-payments/code-server/pkg/cluster"
	clustertests "github.com/code-payments/code-server/pkg/cluster/tests"
	"github.com/code-payments/code-server/pkg/etcdtest"
)

func TestEtcd(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	clustertests.RunClusterTests(t, func() (cluster.Cluster, error) {
		return NewCluster(client, fmt.Sprintf("/cluster-%s", uuid.New().String()), 10*time.Second), nil
	})
}

func TestInvalidValues(t *testing.T) {
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	c := NewCluster(client, "/cluster-invalid", 10*time.Second)

	w := c.WatchMembers(ctx)
	<-w

	mem, err := c.CreateMembership()
	require.NoError(err)
	require.NoError(mem.SetData("hello"))
	require.NoError(mem.Register(context.Background()))

	expected := []cluster.Member{{ID: mem.ID(), Data: mem.Data()}}
	require.Equal(expected, <-w)

	get, err := client.Get(ctx, fmt.Sprintf("/cluster-invalid/%s", mem.ID()), v3.WithPrefix())
	require.NoError(err)
	require.Len(get.Kvs, 1)

	// Putting an invalid record _will_ trigger a refresh (set change), _but_
	// due the value should be dropped/ignored.
	_, err = client.Put(ctx, fmt.Sprintf("/cluster-invalid/%s", uuid.New().String()), "hello")
	require.NoError(err)

	require.Equal(expected, <-w)
}
