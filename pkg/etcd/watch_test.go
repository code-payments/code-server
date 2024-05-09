package etcd

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/code-payments/code-server/pkg/etcdtest"
)

func TestWatch(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = client.Put(ctx, "/my/tree/one", "1")
	require.NoError(err)
	_, err = client.Put(ctx, "/my/tree/two", "2")
	require.NoError(err)

	var statesMu sync.Mutex
	var snapshots []Snapshot[string, int64]

	ch := WatchPrefix(
		ctx,
		client,
		"/my/tree",
		func(k, v []byte) (string, int64, error) {
			keyParts := bytes.Split(k, []byte("/"))
			key := string(keyParts[len(keyParts)-1])
			val, err := strconv.ParseInt(string(v), 10, 64)
			if err != nil {
				return key, 0, err
			}
			return key, val, nil
		},
	)
	go func() {
		for snapshot := range ch {
			statesMu.Lock()
			snapshots = append(snapshots, snapshot)
			statesMu.Unlock()
		}
	}()

	require.Eventually(func() bool {
		statesMu.Lock()
		defer statesMu.Unlock()
		return len(snapshots) > 0
	}, 2*time.Second, 50*time.Millisecond)

	_, err = client.Delete(ctx, "/my/tree/two")
	require.NoError(err)
	_, err = client.Put(ctx, "/my/tree/one", "10")
	require.NoError(err)
	_, err = client.Put(ctx, "/my/tree/three", "3")
	require.NoError(err)

	require.Eventually(func() bool {
		statesMu.Lock()
		defer statesMu.Unlock()
		return len(snapshots) > 0
	}, 2*time.Second, 50*time.Millisecond)

	expected := []map[string]int64{
		{
			"one": 1,
			"two": 2,
		},
		{
			"one": 1,
		},
		{
			"one": 10,
		},
		{
			"one":   10,
			"three": 3,
		},
	}

	for i := range expected {
		require.True(maps.Equal(expected[i], snapshots[i].Tree))
	}
}
