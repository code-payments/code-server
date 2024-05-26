package grpc

import (
	"context"
)

type contextKey int

const (
	contextKeyRouting contextKey = iota
	contextKeyNode
)

func WithRoutingKey(parent context.Context, val []byte) context.Context {
	return context.WithValue(parent, contextKeyRouting, val)
}

func RoutingKey(ctx context.Context) (val []byte, ok bool) {
	val, ok = ctx.Value(contextKeyRouting).([]byte)
	return
}
