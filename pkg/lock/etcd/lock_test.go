//go:build integration

package etcd

import (
	"context"
	"fmt"
	"github.com/ory/dockertest/v3/docker"
	v3 "go.etcd.io/etcd/client/v3"
	"strings"
	"testing"
	"time"

	"github.com/code-payments/code-server/pkg/etcdtest"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
)

func TestLock(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, teardown, err := etcdtest.StartEtcd(pool)
	require.NoError(err)
	defer teardown()

	for _, tc := range []struct {
		name string
		f    func(t *testing.T, client *v3.Client)
	}{
		{name: "Happy", f: testHappy},
		{name: "MultipleManagers", f: testMultipleManagers},
		{name: "Cancellation", f: testCancellation},
		{name: "Close", f: testClose},
		{name: "DoubleAcquire", f: testDoubleAcquire},
		{name: "DoubleUnlock", f: testDoubleUnlock},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.f(t, client) })
	}
}

func TestLeaderLost(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, nodes, teardown, err := etcdtest.StartEtcdCluster(pool, 3, false)
	require.NoError(err)
	defer teardown()

	members, err := client.Cluster.MemberList(context.Background())
	require.NoError(err)

	lockTTL := 3 * time.Second
	m, err := NewLockManager(client, "/locks", lockTTL, "lm")
	require.NoError(err)
	defer m.Close()

	lock, err := m.Create(context.Background(), "my_lock")
	require.NoError(err)
	lostCh, err := lock.Acquire(context.Background())
	require.NoError(err)

	//
	// Step 1: Look for the current leader, and kill it
	//
	t.Logf("Stopping current leader")
	status, err := client.Status(context.Background(), client.Endpoints()[0])
	require.NoError(err)

	leaderName := ""
	for i := range members.Members {
		if members.Members[i].ID == status.Leader {
			leaderName = members.Members[i].Name
			break
		}
	}

	killed := -1
	require.NotEmpty(leaderName)
	for i, n := range nodes {
		// There's sometimes a path in the container name (/), which is...weird...
		if strings.HasSuffix(n.Container.Name, leaderName) {
			require.NoError(pool.Client.StopContainer(nodes[i].Container.ID, 0))
			killed = i
		}
	}
	require.NotEqual(-1, killed)

	//
	// Step 2: Since we have 3 nodes, a single failure should not disrupt.
	//         Additionally, the leader should move seamlessly.
	//
	waitTime := 3 * lockTTL / 2
	t.Logf("Ensuring lock not lost (waiting %v) (%v)", waitTime, time.Now())
	start := time.Now()
	select {
	case <-lostCh:
		lostCh, err = lock.Acquire(context.Background())
		t.Logf("Lost lock after %v. Indicates leader failover took too long", time.Since(start))
	case <-time.After(waitTime):
		// If we make it past 5 seconds, then it's very likely we held the lock.
		//
		// If we expired at 5 seconds (the ttl), it means that the session wasn't
		// able to hold. This is technically a valid case, as it's all a timing
		// problem. To avoid the timing problem, we expect MoveLeader to be more
		// graceful.
	}

	//
	// Step 3: Kill one of the remaining members, bringing down the cluster (leaderless)
	//
	t.Logf("Stopping another node")
	for i := range nodes {
		if i == killed {
			continue
		}

		require.NoError(pool.Client.StopContainer(nodes[i].Container.ID, 0))
		break
	}

	// Our lock should be notified of a loss
	t.Logf("Waiting for lost propagation")
	<-lostCh
	t.Logf("Resuming cluster")

	//
	// Step 4: Restart cluster
	//
	for _, n := range nodes {
		err := pool.Client.StartContainer(n.Container.ID, &docker.HostConfig{
			AutoRemove: true,
		})
		if err == nil {
			continue
		}

		require.NotErrorIs(err, &docker.ContainerAlreadyRunning{})
	}

	lock, err = m.Create(context.Background(), "my-lock")
	require.NoError(err)

	t.Log("Acquiring a lock (with healthy cluster)")
	_, err = lock.Acquire(context.Background())
	require.NoError(err)

	require.NoError(lock.Unlock(context.Background()))
}

func TestMoveLeader(t *testing.T) {
	require := require.New(t)

	pool, err := dockertest.NewPool("")
	require.NoError(err)

	client, nodes, teardown, err := etcdtest.StartEtcdCluster(pool, 3, true)
	require.NoError(err)
	defer teardown()

	members, err := client.Cluster.MemberList(context.Background())
	require.NoError(err)

	m, err := NewLockManager(client, "/locks", 3*time.Second, "lm")
	require.NoError(err)
	defer m.Close()

	lock, err := m.Create(context.Background(), "my_lock")
	require.NoError(err)
	lostCh, err := lock.Acquire(context.Background())
	require.NoError(err)

	status, err := client.Status(context.Background(), client.Endpoints()[0])
	require.NoError(err)

	newLeader := uint64(0)
	var leaderClient *v3.Client

	for _, m := range members.Members {
		if newLeader == 0 && m.ID != status.Leader {
			newLeader = m.ID
			continue
		}

		if m.ID == status.Leader {
			for _, n := range nodes {
				// There's sometimes a path in the container name (/), which is...weird...
				if strings.HasSuffix(n.Container.Name, m.Name) {
					leaderClient, err = v3.NewFromURL(
						fmt.Sprintf("localhost:%s", n.GetPort("2379/tcp")),
					)
					require.NoError(err)
				}
			}
		}
	}

	_, err = leaderClient.MoveLeader(context.Background(), newLeader)
	require.NoError(err)

	t.Log("Ensuring not lost")

	select {
	case <-lostCh:
		require.Fail("should not have lost lock")
	case <-time.After(6 * time.Second):
	}
}

func testHappy(t *testing.T, client *v3.Client) {
	require := require.New(t)

	lm, err := NewLockManager(client, "/locks", 10*time.Second, "lock-manager")
	require.NoError(err)
	defer lm.Close()

	lock, err := lm.Create(context.Background(), "my_lock")
	require.NoError(err)
	require.False(lock.IsLocked())

	acquired := make(chan struct{})

	go func() {
		lostCh, err := lock.Acquire(context.Background())
		require.NoError(err)

		acquired <- struct{}{}
		<-lostCh
		close(acquired)
	}()

	<-acquired
	require.True(lock.IsLocked())

	kvs, err := client.Get(context.Background(), "/locks/my_lock", v3.WithPrefix())
	require.NoError(err)
	require.Len(kvs.Kvs, 1)
	require.Equal("lock-manager", string(kvs.Kvs[0].Value))
	require.NoError(lock.Unlock(context.Background()))
	<-acquired

	require.False(lock.IsLocked())
}

func testMultipleManagers(t *testing.T, client *v3.Client) {
	require := require.New(t)

	var err error
	managers := make([]*LockManager, 2)
	for i := 0; i < 2; i++ {
		managers[i], err = NewLockManager(client, "/locks", 10*time.Second, fmt.Sprintf("lm-%d", i))
		require.NoError(err)
		defer managers[i].Close()
	}

	type state int
	const (
		statePending state = iota
		stateLocked
		stateLost
	)

	type event struct {
		managerID int
		state     state
	}

	eventCh := make(chan event, 8)

	lockContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := range managers {
		go func(id int) {
			lock, err := managers[id].Create(context.Background(), "my_lock")
			require.NoError(err)

			eventCh <- event{managerID: id, state: statePending}
			lostCh, err := lock.Acquire(lockContext)
			require.NoError(err)

			eventCh <- event{managerID: id, state: stateLocked}
			<-lostCh
			eventCh <- event{managerID: id, state: stateLost}
		}(i)
	}

	e := <-eventCh
	require.Equal(statePending, e.state)

	e = <-eventCh
	require.Equal(statePending, e.state)

	e = <-eventCh
	require.Equal(stateLocked, e.state)

	// A bit race-y, but that's fine
	select {
	case <-eventCh:
		require.FailNow("state should be stable")
	case <-time.After(2 * time.Second):
	}

	// At this point, we expect two independent locks from each of the managers.
	// The lock holder (i.e. the one who emitted stateLock) should be the current owner.
	kvs, err := client.Get(context.Background(), "/locks/my_lock", v3.WithPrefix(), v3.WithSort(v3.SortByCreateRevision, v3.SortAscend))
	require.NoError(err)
	require.Len(kvs.Kvs, 2)

	require.Equal(fmt.Sprintf("lm-%d", e.managerID), string(kvs.Kvs[0].Value))

	// LockManagers produce re-entrant locks (for better or worse)
	lock, err := managers[e.managerID].Create(context.Background(), "my_lock")
	require.NoError(err)

	// This should _not_ block!
	lostCh, err := lock.Acquire(context.Background())
	require.NoError(err)

	require.NoError(lock.Unlock(context.Background()))
	<-lostCh

	previousManager := e.managerID

	fmt.Println("Unlocked and acknowledged...waiting for events")

	e1 := <-eventCh
	e2 := <-eventCh

	if e1.managerID == previousManager {
		require.Equal(stateLost, e1.state)
		require.Equal(stateLocked, e2.state)
	} else {
		require.Equal(stateLocked, e1.state)
		require.Equal(stateLost, e2.state)
	}
}

func testCancellation(t *testing.T, client *v3.Client) {
	require := require.New(t)

	lm, err := NewLockManager(client, "/locks", 10*time.Second, "lock-manager")
	require.NoError(err)
	defer lm.Close()

	lock, err := lm.Create(context.Background(), "my_lock")
	require.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lostCh, err := lock.Acquire(ctx)
	require.NoError(err)
	cancel()

	<-lostCh

	_, err = lock.Acquire(ctx)
	require.ErrorIs(err, context.Canceled)
}

func testClose(t *testing.T, client *v3.Client) {
	require := require.New(t)

	lm, err := NewLockManager(client, "/locks", 10*time.Second, "lock-manager")
	require.NoError(err)
	defer lm.Close()

	lock, err := lm.Create(context.Background(), "my_lock")
	require.NoError(err)

	lostCh, err := lock.Acquire(context.Background())
	require.NoError(err)

	lm.Close()
	<-lostCh

	_, err = lock.Acquire(context.Background())
	require.ErrorContains(err, "closed")

	lock, err = lm.Create(context.Background(), "my_lock")
	require.Nil(lock)
	require.ErrorContains(err, "closed")
}

func testDoubleAcquire(t *testing.T, client *v3.Client) {
	require := require.New(t)

	lm, err := NewLockManager(client, "/locks", 10*time.Second, "lock-manager")
	require.NoError(err)
	defer lm.Close()

	lock, err := lm.Create(context.Background(), "my_lock")
	require.NoError(err)

	acquired := make(chan struct{})
	go func() {
		lostCh, err := lock.Acquire(context.Background())
		require.NoError(err)
		acquired <- struct{}{}
		<-lostCh
		close(acquired)
	}()

	<-acquired
	_, err = lock.Acquire(context.Background())
	require.ErrorContains(err, "concurrently")
}

func testDoubleUnlock(t *testing.T, client *v3.Client) {
	require := require.New(t)

	lm, err := NewLockManager(client, "/locks", 10*time.Second, "lock-manager")
	require.NoError(err)
	defer lm.Close()

	lock, err := lm.Create(context.Background(), "my_lock")
	require.NoError(err)

	lostCh, err := lock.Acquire(context.Background())

	require.NoError(err)
	require.NoError(lock.Unlock(context.Background()))
	require.NoError(lock.Unlock(context.Background()))

	<-lostCh
}
