package grpc

import (
	"math/rand"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/code-payments/code-server/pkg/cluster/registry"
	"github.com/code-payments/code-server/pkg/cluster/ring"
)

type picker struct {
	nodes     []*registry.NodeDescriptor
	subConns  map[string]balancer.SubConn
	tokenRing *ring.HashRing
}

func newPicker(
	nodes []*registry.NodeDescriptor,
	subConns map[balancer.SubConn]*registry.NodeDescriptor,
	tokenRing *ring.HashRing,
) *picker {
	p := &picker{
		nodes:     nodes,
		subConns:  make(map[string]balancer.SubConn),
		tokenRing: tokenRing,
	}

	for sc, n := range subConns {
		p.subConns[n.ID] = sc
	}

	return p
}

func (p *picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	var selectedNodeID string
	if routingKey, ok := RoutingKey(info.Ctx); ok {
		selectedNodeID = p.tokenRing.GetNode(routingKey)
	} else {
		selectedNodeID = p.getSimpleNode()
	}

	if selectedNodeID == "" {
		return balancer.PickResult{}, status.Error(codes.Unavailable, "no endpoint available")
	}

	sc, ok := p.subConns[selectedNodeID]
	if !ok {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	return balancer.PickResult{SubConn: sc}, nil
}

func (p *picker) getSimpleNode() string {
	if len(p.nodes) == 0 {
		return ""
	}

	totalWeight := uint32(0)
	for _, n := range p.nodes {
		totalWeight += n.Weight
	}
	if totalWeight == 0 {
		return ""
	}

	accumWeight := uint32(0)
	targetWeight := uint32(rand.Int31n(int32(totalWeight)))
	for _, n := range p.nodes {
		accumWeight += n.Weight
		if targetWeight < accumWeight {
			return n.ID
		}
	}

	return ""
}
