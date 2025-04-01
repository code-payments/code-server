package example

// Example uses deprecated service

/*
import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	invitepb "github.com/code-payments/code-protobuf-api/generated/go/invite/v2"

	cgrpc "github.com/code-payments/code-server/pkg/cluster/grpc"
	"github.com/code-payments/code-server/pkg/cluster/registry"
)

type ClusteredInviteService struct {
	nodeID                   string
	serverID                 int
	registry                 registry.Observer
	forwardingClientProvider func() invitepb.InviteClient

	closeFn sync.Once
	closeCh chan struct{}

	cacheMu sync.Mutex
	cache   map[string]struct{}

	invitepb.UnimplementedInviteServer
}

func NewClusteredInviteService(
	nodeID string,
	serverID int,
	registry registry.Observer,
	forwardingClientProvider func() invitepb.InviteClient,
) *ClusteredInviteService {
	s := &ClusteredInviteService{
		nodeID:                   nodeID,
		serverID:                 serverID,
		registry:                 registry,
		forwardingClientProvider: forwardingClientProvider,

		closeCh: make(chan struct{}),

		cache: make(map[string]struct{}),
	}

	go s.cacheLoop()

	return s
}

func (c *ClusteredInviteService) GetInviteCount(ctx context.Context, req *invitepb.GetInviteCountRequest) (*invitepb.GetInviteCountResponse, error) {
	mismatch := c.registry.Owner(req.UserId.Value) != c.nodeID
	if mismatch {
		if fc := c.forwardingClientProvider(); fc != nil {
			resp, err := fc.GetInviteCount(cgrpc.WithRoutingKey(ctx, req.UserId.Value), req)
			if err != nil {
				return nil, err
			}

			return &invitepb.GetInviteCountResponse{
				InviteCount: resp.InviteCount,
			}, nil
		}
	}

	cacheKey := string(req.UserId.Value)
	c.cacheMu.Lock()
	_, cached := c.cache[cacheKey]
	c.cache[cacheKey] = struct{}{}
	c.cacheMu.Unlock()

	result := NewResult(req.UserId, c.serverID, mismatch, cached)
	return &invitepb.GetInviteCountResponse{
		InviteCount: uint32(result),
	}, nil
}

func (c *ClusteredInviteService) Close() {
	c.closeFn.Do(func() {
		close(c.closeCh)
	})
}

// cacheLoop is an example of keeping the local state eventually consistent
// with the cluster state. This use case is particularly notable if we have
// caching (though beware of inconsistency) or user sessions (where we would
// want to force them to reconnect on change).
func (c *ClusteredInviteService) cacheLoop() {
	// The loop _may_ be expensive, so we take care to not block the WatchNodes()
	// call. A buffer of 1 _should_ suffice since we don't care about the actual set change.
	//
	// If we want to be _extra_ paranoid, we can use a ticker for eventual consistency.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-c.closeCh
		cancel()
	}()

	changeCh := make(chan struct{}, 1)
	go func() {
		membershipChangeCh := c.registry.WatchNodes(ctx)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-membershipChangeCh:
			case <-ticker.C:
			}

			select {
			case changeCh <- struct{}{}:
			default:
			}
		}
	}()

	for {
		select {
		case <-c.closeCh:
			return
		case <-changeCh:
		}

		c.cacheMu.Lock()
		for k := range c.cache {
			if c.registry.Owner([]byte(k)) != c.nodeID {
				delete(c.cache, k)
			}
		}
		c.cacheMu.Unlock()
	}
}

type ClusteredInviteClient struct {
	client invitepb.InviteClient
}

func NewClusteredInviteClient(cc grpc.ClientConnInterface) *ClusteredInviteClient {
	return &ClusteredInviteClient{client: invitepb.NewInviteClient(cc)}
}

func (c *ClusteredInviteClient) GetInviteCount(ctx context.Context, in *invitepb.GetInviteCountRequest, opts ...grpc.CallOption) (*invitepb.GetInviteCountResponse, error) {
	return c.client.GetInviteCount(cgrpc.WithRoutingKey(ctx, in.GetUserId().Value), in, opts...)
}

func (c *ClusteredInviteClient) InvitePhoneNumber(ctx context.Context, in *invitepb.InvitePhoneNumberRequest, opts ...grpc.CallOption) (*invitepb.InvitePhoneNumberResponse, error) {
	panic("implement me")
}

func (c *ClusteredInviteClient) GetInvitationStatus(ctx context.Context, in *invitepb.GetInvitationStatusRequest, opts ...grpc.CallOption) (*invitepb.GetInvitationStatusResponse, error) {
	panic("implement me")
}

type Result uint32

func NewResult(userID *commonpb.UserId, serverID int, mismatch, cached bool) Result {
	var m uint32
	if mismatch {
		m = 1
	}
	if cached {
		m |= 2
	}

	return Result(uint32(userID.Value[0]) | uint32((serverID&0xff)<<8) | m<<16)
}

func (r Result) GetUserID() []byte {
	return []byte{byte(r & 0xff)}
}

func (r Result) GetServerIndex() int {
	return int((r >> 8) & 0xff)
}

func (r Result) Mismatch() bool {
	return (r>>16)&0x01 == 1
}

func (r Result) Cached() bool {
	return (r>>16)&0x02 == 2
}
*/
