package grpc

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"

	"github.com/code-payments/code-server/pkg/cluster/registry"
	"github.com/code-payments/code-server/pkg/cluster/ring"
)

const (
	BalancerName   = "cluster"
	BalancerConfig = `{"loadBalancingConfig": [{"cluster": {}}]}`
)

func RegisterLoadBalancer(registry registry.Reader) {
	balancer.Register(&balancerBuilder{
		log:    logrus.WithField("type", "cluster/grpc/balancer"),
		reader: registry,
	})
}

type balancerBuilder struct {
	log    *logrus.Entry
	reader registry.Reader
}

func (b *balancerBuilder) Name() string {
	return BalancerName
}

func (b *balancerBuilder) Build(cc balancer.ClientConn, _ balancer.BuildOptions) balancer.Balancer {
	return &sbBalancer{
		log:    b.log,
		reader: b.reader,
		cc:     cc,

		subConns: make(map[string]balancer.SubConn),
		scStates: make(map[balancer.SubConn]balancer.SubConnState),
		scNodes:  make(map[balancer.SubConn]*registry.NodeDescriptor),
	}
}

type sbBalancer struct {
	log    *logrus.Entry
	reader registry.Reader
	cc     balancer.ClientConn

	picker balancer.Picker

	mu       sync.RWMutex
	subConns map[string]balancer.SubConn
	scStates map[balancer.SubConn]balancer.SubConnState
	scNodes  map[balancer.SubConn]*registry.NodeDescriptor
}

func (b *sbBalancer) UpdateClientConnState(state balancer.ClientConnState) error {
	tokenRing := state.ResolverState.Attributes.Value(ringAttributeKey).(*ring.HashRing)
	if tokenRing == nil {
		return fmt.Errorf("missing ring: balancer must be used with resolver")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	addressSet := make(map[string]struct{})

	for _, addr := range state.ResolverState.Addresses {
		node, ok := addr.Attributes.Value(contextKeyNode).(*registry.NodeDescriptor)
		if !ok {
			b.log.Warn("NodeDescriptor not present in metadata, ignoring", zap.String("address", addr.Addr))
			continue
		}

		addressSet[addr.Addr] = struct{}{}

		// If we already have a SubConn for the address, then we're simply updating
		// the NodeDescriptor state. The picker will deal with any routing level updates.
		if sc, ok := b.subConns[addr.Addr]; ok {
			b.scNodes[sc] = node
			continue
		}

		// Set up a new SubConn for the newly observed Node.
		sc, err := b.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{})
		if err != nil {
			b.log.Warn("Failed to create new SubConn", zap.Error(err), zap.String("address", addr.Addr))
			continue
		}

		b.log.Debug("Created new SubConn", zap.String("address", addr.Addr))

		b.subConns[addr.Addr] = sc
		b.scStates[sc] = balancer.SubConnState{ConnectivityState: connectivity.Idle}
		b.scNodes[sc] = node

		// We want to immediately start the connection flow to help reduce 'cold starts' on actual requests.
		//
		// Note: In a multi-service world, we would only want to do this on services that we've actually communicated
		// with in the past to reduce the number of overall connections. However, cluster sizes should be relatively
		// small and homogeneous.
		sc.Connect()
	}

	// Prune any SubConn's for nodes that are no longer resolved.
	for addr, sc := range b.subConns {
		if _, ok := addressSet[addr]; !ok {
			// Note: The reason we're deleting from subConns and _not_ scStates/scNodes is
			// that they're required to correctly transition the state into Shutdown.
			//
			// See UpdateSubConnState() below for their transition.
			delete(b.subConns, addr)

			sc.Shutdown()
		}
	}

	b.regeneratePicker(tokenRing)

	// We just report the Balancer as ready (which in turn indicates that the ClientConn is ready)
	// as we don't do any connection warming or probing.
	//
	// In theory, this should only affect Dial() when `WithBlock()` is used. This behaviour
	// effectively makes the `WithBlock()` a no-op.
	b.cc.UpdateState(balancer.State{Picker: b.picker, ConnectivityState: connectivity.Ready})
	return nil
}

func (b *sbBalancer) UpdateSubConnState(sc balancer.SubConn, newState balancer.SubConnState) {
	b.mu.Lock()
	defer b.mu.Unlock()

	oldState, ok := b.scStates[sc]
	if !ok {
		b.log.
			WithField("new_state", newState.ConnectivityState.String()).
			Warn("State change for unknown SubConn")

		return
	}
	b.scStates[sc] = newState

	node, ok := b.scNodes[sc]
	if !ok {
		b.log.Warn("NodeDescriptor doesn't exist for SubConn")
		return
	}

	address := fmt.Sprintf("%s:%d", node.Address.IP, node.Address.Port)
	switch newState.ConnectivityState {
	case connectivity.Idle:
		b.log.WithField("address", address).Debug("SubConn changed to idle; reconnecting")
		sc.Connect()
	case connectivity.Shutdown:
		b.log.
			WithField("address", address).
			WithError(newState.ConnectionError).
			Debug("SubConn shutting down")

		delete(b.scStates, sc)
		delete(b.scNodes, sc)
	}

	// There's a slightly sinister case where the Picker may block if the selected
	// SubConn is non-ready. In this case, it will wait until UpdateState() is called
	// again (by us, the LoadBalancer).
	//
	// To help avoid/unblock this case, we poke the picker (via UpdateState) every time
	// we see a state transition.
	//
	// This behavior is acceptable in small homogeneous clusters, but maybe revised in
	// larger deploys (by service).
	if (oldState.ConnectivityState == connectivity.Ready) != (newState.ConnectivityState == connectivity.Ready) {
		b.cc.UpdateState(balancer.State{Picker: b.picker, ConnectivityState: connectivity.Ready})
	}
}

func (b *sbBalancer) ResolverError(err error) {
	b.log.WithError(err).Error("Unexpected resolver error")
}

func (b *sbBalancer) Close() {
}

func (b *sbBalancer) regeneratePicker(tokenRing *ring.HashRing) {
	b.log.WithField("nodes", len(b.scNodes)).Debug("Regenerating picker")

	nodes := make([]*registry.NodeDescriptor, 0, len(b.scNodes))
	for _, node := range b.scNodes {
		nodes = append(nodes, node)
	}

	b.picker = newPicker(nodes, b.scNodes, tokenRing)
}
