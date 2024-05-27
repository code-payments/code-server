package example

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/nettest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	invitepb "github.com/code-payments/code-protobuf-api/generated/go/invite/v2"

	cgrpc "github.com/code-payments/code-server/pkg/cluster/grpc"
	"github.com/code-payments/code-server/pkg/cluster/memory"
	"github.com/code-payments/code-server/pkg/cluster/registry"
)

type testServer struct {
	lis     net.Listener
	serv    *grpc.Server
	node    registry.Node
	service *ClusteredInviteService
}

func (t *testServer) close() {
	if node := t.node; node != nil {
		_ = node.Deregister(context.Background())
	}
	if serv := t.serv; serv != nil {
		serv.GracefulStop()
	}
	if l := t.lis; l != nil {
		_ = l.Close()
	}
	if service := t.service; service != nil {
		service.Close()
	}
}

func setupServers(
	require *require.Assertions,
	r *registry.ClusteredRegistry,
	n int,
	clientProvider func() invitepb.InviteClient,
) ([]*testServer, func()) {
	servers := make([]*testServer, n)
	cleanup := func() {
		for _, server := range servers {
			server.close()
		}
	}

	for i := range servers {
		lis, err := nettest.NewLocalListener("tcp")
		require.NoError(err)

		node, err := r.NewNode(lis.Addr().String())
		require.NoError(err)
		require.NoError(node.SetWeight(500))

		require.NoError(node.Register(context.Background()))

		servers[i] = &testServer{
			lis:  lis,
			serv: grpc.NewServer(),
			node: node,
			service: NewClusteredInviteService(
				node.ID(),
				i,
				r,
				clientProvider,
			),
		}

		invitepb.RegisterInviteServer(servers[i].serv, servers[i].service)

		go func(i int) {
			_ = servers[i].serv.Serve(lis)
		}(i)
	}

	require.Eventually(func() bool {
		nodes, _ := r.GetNodes()
		return len(nodes) == n
	}, 5*time.Second, 10*time.Millisecond)

	return servers, cleanup
}

func TestNoForwardRouting(t *testing.T) {
	require := require.New(t)

	cluster := memory.NewCluster()
	registry := registry.NewClusteredRegistry(cluster)

	// Deployments using the gRPC cluster should install these globally.
	cgrpc.RegisterResolver(registry)
	cgrpc.RegisterLoadBalancer(registry)
	servers, cleanup := setupServers(require, registry, 5, func() invitepb.InviteClient {
		return nil
	})
	defer cleanup()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	cc, err := grpc.Dial(
		"cluster://authority/endpoint",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(cgrpc.BalancerConfig),
	)
	require.NoError(err)

	inviteClient := invitepb.NewInviteClient(cc)

	for u := 0; u < 20; u++ {
		userID := &commonpb.UserId{Value: []byte{byte(u)}}

		//
		// Test 1: No routing key is provided, and so falls back to round-robin
		//
		requests := 1000
		serverHits := map[int]int{}
		if true {
			for i := 0; i < requests; i++ {
				resp, err := inviteClient.GetInviteCount(context.Background(), &invitepb.GetInviteCountRequest{
					UserId: userID,
				})
				require.NoError(err)

				result := Result(resp.InviteCount)
				serverHits[result.GetServerIndex()]++
			}

			require.Greater(len(serverHits), 2)
			for _, hits := range serverHits {
				require.InDelta(
					1/float64(len(servers)),
					float64(hits)/float64(requests),
					0.05,
				)
			}
		}

		//
		// Test 2: Routing key is provided, so we expect it to hit the same server every time.
		//
		clear(serverHits)
		ctx := cgrpc.WithRoutingKey(context.Background(), userID.Value)
		mismatches := 0
		for i := 0; i < 10; i++ {
			resp, err := inviteClient.GetInviteCount(ctx, &invitepb.GetInviteCountRequest{
				UserId: userID,
			})
			require.NoError(err)

			result := Result(resp.InviteCount)
			require.False(result.Mismatch())
			if i > 0 {
				require.True(result.Cached())
			}
			serverHits[result.GetServerIndex()]++
		}

		require.Len(serverHits, 1)
		require.Zero(mismatches)
	}

	fmt.Println("done")
}

func TestForwardRouting(t *testing.T) {
	require := require.New(t)

	cluster := memory.NewCluster()
	registry := registry.NewClusteredRegistry(cluster)

	// Deployments using the gRPC cluster should install these globally.
	cgrpc.RegisterResolver(registry)
	cgrpc.RegisterLoadBalancer(registry)

	var forwardingClient invitepb.InviteClient
	clientProvider := func() invitepb.InviteClient {
		return forwardingClient
	}

	servers, cleanup := setupServers(require, registry, 2, clientProvider)
	defer cleanup()

	cc, err := grpc.Dial(
		"cluster://authority/endpoint",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(cgrpc.BalancerConfig),
	)
	require.NoError(err)

	forwardingClient = invitepb.NewInviteClient(cc)

	userID := &commonpb.UserId{Value: []byte{20}}
	owner := registry.Owner(userID.Value)
	ownerIndex := 0
	for _, s := range servers {
		if s.service.nodeID == owner {
			ownerIndex = s.service.serverID
		} else {
			cc, err = grpc.Dial(
				s.lis.Addr().String(),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
		}
	}

	require.NotNil(cc)
	require.NoError(err)

	client := invitepb.NewInviteClient(cc)
	resp, err := client.GetInviteCount(context.Background(), &invitepb.GetInviteCountRequest{
		UserId: userID,
	})
	require.NoError(err)

	result := Result(resp.InviteCount)
	require.False(result.Mismatch())
	require.Equal(ownerIndex, result.GetServerIndex())
	require.Equal(userID.Value, result.GetUserID())
}

func TestCacheInvalidation(t *testing.T) {
	require := require.New(t)

	cluster := memory.NewCluster()
	registry := registry.NewClusteredRegistry(cluster)

	// Deployments using the gRPC cluster should install these globally.
	cgrpc.RegisterResolver(registry)
	cgrpc.RegisterLoadBalancer(registry)

	servers, cleanup := setupServers(require, registry, 2, func() invitepb.InviteClient { return nil })
	defer cleanup()

	cc, err := grpc.Dial(
		"cluster://authority/endpoint",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(cgrpc.BalancerConfig),
	)
	require.NoError(err)

	client := invitepb.NewInviteClient(cc)
	userID := &commonpb.UserId{Value: []byte{30}}

	ctx := cgrpc.WithRoutingKey(context.Background(), userID.Value)

	resp, err := client.GetInviteCount(ctx, &invitepb.GetInviteCountRequest{UserId: userID})
	require.NoError(err)
	resp, err = client.GetInviteCount(ctx, &invitepb.GetInviteCountRequest{UserId: userID})
	require.NoError(err)

	result := Result(resp.InviteCount)
	require.True(result.Cached())

	server := servers[result.GetServerIndex()]
	server.service.cacheMu.Lock()
	_, cached := server.service.cache[string(userID.Value)]
	server.service.cacheMu.Unlock()
	require.True(cached)

	require.NoError(servers[result.GetServerIndex()].node.Deregister(ctx))
	require.Eventually(func() bool {
		_, cached = servers[result.GetServerIndex()].service.cache[string(userID.Value)]
		return cached
	}, 5*time.Second, 10*time.Millisecond)
}
