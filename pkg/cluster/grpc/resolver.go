package grpc

import (
	"context"
	"fmt"
	"go.uber.org/zap"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"

	"github.com/code-payments/code-server/pkg/cluster/registry"
	"github.com/code-payments/code-server/pkg/cluster/ring"
)

const (
	Scheme = "cluster"

	ringAttributeKey = "cluster-ring"
)

func RegisterResolver(registry registry.Observer) {
	resolver.Register(&resolverBuilder{
		log:      logrus.WithField("type", "cluster/grpc/registry"),
		registry: registry,
	})
}

type resolverBuilder struct {
	log      *logrus.Entry
	registry registry.Observer
}

func (rb *resolverBuilder) Scheme() string {
	return Scheme
}

func (rb *resolverBuilder) Build(_ resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	r := &registryResolver{
		log:        logrus.StandardLogger().WithField("type", "cluster/grpc/registry"),
		shutdownCh: make(chan struct{}),
		cc:         cc,
		obs:        rb.registry,
	}

	go r.watchEndpoints()

	return r, nil
}

type registryResolver struct {
	log *logrus.Entry
	obs registry.Observer
	cc  resolver.ClientConn

	shutdownCh chan struct{}
}

func (r *registryResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	nodes, err := r.obs.GetNodes()
	if err != nil {
		r.log.WithError(err).Error("Failed to retrieve nodes")
		if err = r.cc.UpdateState(resolver.State{Addresses: nil}); err != nil {
			r.log.Error("Failed to clear local state", zap.Error(err))
		}

		return
	}

	r.resolveNodes(nodes)
}

func (r *registryResolver) Close() {
	close(r.shutdownCh)
}

func (r *registryResolver) watchEndpoints() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-r.shutdownCh
		cancel()
	}()

	for nodes := range r.obs.WatchNodes(ctx) {
		r.resolveNodes(nodes)
	}
}

func (r *registryResolver) resolveNodes(nodes []*registry.NodeDescriptor) {
	claims := map[string]int{}
	addresses := make([]resolver.Address, 0, len(nodes))
	for _, n := range nodes {
		n := n

		if n.Address.IP == "" || n.Address.Port == 0 {
			r.log.WithField("id", n.ID).Warn("Dropping node with missing address")
			continue
		}

		claims[n.ID] = int(n.Weight)
		addresses = append(addresses, resolver.Address{
			Addr:       fmt.Sprintf("%s:%d", n.Address.IP, n.Address.Port),
			Attributes: attributes.New(contextKeyNode, n),
		})
	}

	r.log.Debug("Emitting addresses", zap.Int("addresses", len(addresses)))
	err := r.cc.UpdateState(resolver.State{
		Addresses:  addresses,
		Attributes: attributes.New(ringAttributeKey, ring.NewRingFromMap(claims)),
	})
	if err != nil {
		r.log.Error("Failed to update resolver", zap.Error(err))
	}
}
