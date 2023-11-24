package app

import (
	"google.golang.org/grpc"
)

// Option configures the environment run by Run().
type Option func(o *opts)

type opts struct {
	unaryServerInterceptors  []grpc.UnaryServerInterceptor
	streamServerInterceptors []grpc.StreamServerInterceptor
}

// WithUnaryServerInterceptor configures the app's gRPC server to use the provided interceptor.
//
// Interceptors are evaluated in addition order, and configured interceptors are executed after
// the app's default interceptors.
func WithUnaryServerInterceptor(interceptor grpc.UnaryServerInterceptor) Option {
	return func(o *opts) {
		o.unaryServerInterceptors = append(o.unaryServerInterceptors, interceptor)
	}
}

// WithStreamServerInterceptor configures the app's gRPC server to use the provided interceptor.
//
// Interceptors are evaluated in addition order, and configured interceptors are executed after
// the app's default interceptors.
func WithStreamServerInterceptor(interceptor grpc.StreamServerInterceptor) Option {
	return func(o *opts) {
		o.streamServerInterceptors = append(o.streamServerInterceptors, interceptor)
	}
}
