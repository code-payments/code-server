package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/cluster"
	"github.com/code-payments/code-server/pkg/cluster/ring"
)

type ClusterMember struct {
	desc NodeDescriptor
	m    cluster.Membership
}

func (c *ClusterMember) ID() string {
	return c.m.ID()
}

func (c *ClusterMember) Register(ctx context.Context) error {
	return c.m.Register(ctx)
}

func (c *ClusterMember) Deregister(ctx context.Context) error {
	return c.m.Deregister(ctx)
}

func (c *ClusterMember) SetWeight(weight uint32) error {
	c.desc.Weight = weight
	b, err := json.Marshal(c.desc)
	if err != nil {
		return err
	}

	return c.m.SetData(string(b))
}

func (c *ClusterMember) GetWeight() uint32 {
	return c.desc.Weight
}

type ClusteredRegistry struct {
	log     *logrus.Entry
	cluster cluster.Cluster

	closeFn sync.Once
	closeCh chan struct{}

	nodesMu   sync.RWMutex
	nodes     []*NodeDescriptor
	tokenRing *ring.HashRing
}

func NewClusteredRegistry(cluster cluster.Cluster) *ClusteredRegistry {
	c := &ClusteredRegistry{
		log:       logrus.StandardLogger().WithField("type", "cluster/registry/ClusteredRegistry"),
		cluster:   cluster,
		closeCh:   make(chan struct{}),
		tokenRing: ring.NewHashRing(),
	}

	go c.watch()

	return c
}

func (c *ClusteredRegistry) Close() {
	c.closeFn.Do(func() {
		close(c.closeCh)
	})
}

func (c *ClusteredRegistry) GetNode(id string) (*NodeDescriptor, error) {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()

	for _, n := range c.nodes {
		if n.ID == id {
			return n, nil
		}
	}

	return nil, nil
}

func (c *ClusteredRegistry) GetNodes() ([]*NodeDescriptor, error) {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()

	return c.nodes, nil
}

func (c *ClusteredRegistry) Owner(token []byte) string {
	return c.tokenRing.GetNode(token)
}

func (c *ClusteredRegistry) NewNode(addr string) (Node, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid addr: %s", addr)
	}

	port, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", parts[1], err)
	}

	clusterMember, err := c.cluster.CreateMembership()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster member: %w", err)
	}

	member := &ClusterMember{
		desc: NodeDescriptor{
			ID: clusterMember.ID(),
			Address: NodeAddress{
				IP:   parts[0],
				Port: int(port),
			},
			Weight: 0,
		},
		m: clusterMember,
	}

	return member, nil
}

func (c *ClusteredRegistry) WatchNodes(ctx context.Context) <-chan []*NodeDescriptor {
	ch := make(chan []*NodeDescriptor, 1)

	go func() {
		for members := range c.cluster.WatchMembers(ctx) {
			nodes := make([]*NodeDescriptor, 0, len(members))
			for _, m := range members {
				nd := &NodeDescriptor{}
				if err := json.Unmarshal([]byte(m.Data), &nd); err != nil {
					c.log.
						WithError(err).
						WithField("id", m.ID).
						Warn("Failed to unmarshal node; dropping")

					continue
				}

				nd.ID = m.ID
				nodes = append(nodes, nd)
			}

			ch <- nodes
		}
	}()

	return ch
}

func (c *ClusteredRegistry) watch() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c.closeCh
		cancel()
	}()

	for nodes := range c.WatchNodes(ctx) {
		totalClaims := 0
		claims := make(map[string]int, len(nodes))
		for _, n := range nodes {
			claims[n.ID] = int(n.Weight)
			totalClaims += int(n.Weight)
		}

		tokenRing := ring.NewRingFromMap(claims)

		c.nodesMu.Lock()
		c.nodes = nodes
		c.tokenRing = tokenRing
		c.nodesMu.Unlock()
	}
}
