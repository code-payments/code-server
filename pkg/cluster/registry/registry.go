package registry

import (
	"context"
	"errors"
)

var (
	ErrNodeNotFound = errors.New("endpoint not found")
)

type NodeDescriptor struct {
	ID      string      `json:"id"`
	Address NodeAddress `json:"address"`
	Weight  uint32      `json:"weight,omitempty"`
}

type NodeAddress struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type Node interface {
	// ID returns the Nodes ID.
	ID() string

	// Register registers the endpoint with zero weighting.
	//
	// The endpoint should be visible to other processes after registering.
	Register(ctx context.Context) error

	// Deregister removes the endpoint from the registry.
	//
	// The endpoint should not be visible to other processes after de-registering.
	Deregister(ctx context.Context) error

	// SetWeight sets the weight (without delay) of the endpoint.
	SetWeight(weight uint32) error

	// GetWeight returns the current weight of the endpoint.
	GetWeight() uint32
}

type Observer interface {
	Reader
	Watcher
}

type Reader interface {
	// GetNode returns the node descriptor corresponding to the id.
	//
	// If there is no endpoint registered with the provided ID, ErrNodeNotFound is returned.
	GetNode(id string) (*NodeDescriptor, error)

	// GetNodes returns the set of services that are currently registered.
	GetNodes() ([]*NodeDescriptor, error)

	// Owner returns the Node.ID of the node that owns the specified token.
	//
	// If no owner exists, an empty string is returned.
	Owner(token []byte) string
}

type Watcher interface {
	// WatchNodes returns a channel that emits the entire set of nodes when
	// a change occurs. The channel is closed when the observer is closed,
	// or when the provided context is cancelled.
	WatchNodes(ctx context.Context) <-chan []*NodeDescriptor
}
