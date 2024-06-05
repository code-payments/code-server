package tests

import (
	"context"
	"fmt"
	"github.com/code-payments/code-server/pkg/cluster"
	"github.com/stretchr/testify/require"
	"slices"
	"strings"
	"testing"
)

func RunClusterTests(t *testing.T, ctor func() (cluster.Cluster, error)) {
	for _, tc := range []struct {
		name string
		tf   func(*testing.T, cluster.Cluster)
	}{
		{name: "TestHappyPath", tf: testHappyPath},
		{name: "TestWatchers", tf: testWatchers},
	} {
		cluster, err := ctor()
		require.NoError(t, err)
		t.Run(tc.name, func(t *testing.T) { tc.tf(t, cluster) })
		cluster.Close()
	}
}

func testHappyPath(t *testing.T, c cluster.Cluster) {
	ctx := context.Background()
	require := require.New(t)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := c.WatchMembers(watchCtx)

	awaitMembers := func() []cluster.Member {
		watchMembers := <-w
		getMembers, err := c.GetMembers(ctx)
		require.NoError(err)

		require.Equal(watchMembers, getMembers)
		return watchMembers
	}

	//
	// Step 1: Initialize memberships (but don't register)
	//
	memberships := make([]cluster.Membership, 10)
	for i := range memberships {
		membership, err := c.CreateMembership()
		require.NoError(err)

		memberships[i] = membership
	}
	slices.SortFunc(memberships, func(a, b cluster.Membership) int {
		return strings.Compare(a.ID(), b.ID())
	})

	//
	// Step 2: Incrementally add a member (based on sorted ID), ensure it appears.
	//
	expectedMembers := make([]cluster.Member, 0)
	require.Equal(expectedMembers, awaitMembers())

	for i, m := range memberships {
		require.NoError(m.SetData(fmt.Sprintf("node-%d", i)))
		expectedMembers = append(expectedMembers, cluster.Member{
			ID:   m.ID(),
			Data: m.Data(),
		})

		require.NoError(m.Register(context.Background()))
		require.NoError(m.Register(context.Background())) // noop
		require.Equal(expectedMembers, awaitMembers())
	}

	//
	// Step 3: Modify the data
	//
	for i, m := range memberships {
		require.NoError(m.SetData(fmt.Sprintf("node-%d", 10+i)))
		expectedMembers[i].Data = m.Data()
		require.Equal(expectedMembers, awaitMembers())
	}

	//
	// Step 3: Deregister each membership
	//
	for len(memberships) > 0 {
		require.NoError(memberships[0].Deregister(ctx))
		require.NoError(memberships[0].Deregister(ctx)) // noop
		expectedMembers = expectedMembers[1:]
		memberships = memberships[1:]

		require.Equal(expectedMembers, awaitMembers())
	}
}

type watch struct {
	ctx    context.Context
	cancel func()
	ch     <-chan []cluster.Member
}

func testWatchers(t *testing.T, c cluster.Cluster) {
	ctx := context.Background()
	require := require.New(t)

	watches := make([]watch, 5)
	for i := range watches {
		watchCtx, cancel := context.WithCancel(ctx)
		watches[i] = watch{
			ctx:    watchCtx,
			cancel: cancel,
			ch:     c.WatchMembers(watchCtx),
		}
		require.Empty(<-watches[i].ch)
	}

	waitClosed := func(ids []int) {
		for {
			closed := 0
			for _, id := range ids {
				if _, ok := <-watches[id].ch; !ok {
					closed++
				}
			}
			if closed == len(ids) {
				return
			}

		}
	}

	member, err := c.CreateMembership()
	require.NoError(err)
	require.NoError(member.Register(ctx))

	for i := range watches {
		require.NotEmpty(<-watches[i].ch)
	}

	for i := range watches {
		if i%2 == 0 {
			watches[i].cancel()
		}
	}

	waitClosed([]int{0, 2, 4})

	require.NoError(member.SetData("my data"))

	for i := range watches {
		if i%2 == 0 {
			_, ok := <-watches[i].ch
			require.False(ok)
		} else {
			require.NotEmpty(<-watches[i].ch)
		}
	}

	c.Close()

	require.NoError(member.SetData("my data"))

	for i := range watches {
		_, ok := <-watches[i].ch
		require.False(ok, watches[i].ch)
	}
}
