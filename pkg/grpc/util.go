package grpc

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	healthCheckEndpoint = "/grpc.health.v1.Health/Check"
	fullMethodNameRegex = regexp.MustCompile("/([a-zA-Z0-9]+\\.)+[a-zA-Z0-9]+/[a-zA-Z0-9]+")
)

// ParseFullMethodName parses a gRPC full method name into its components
func ParseFullMethodName(fullMethodName string) (packageName, serviceName, methodName string, err error) {
	if !fullMethodNameRegex.Match([]byte(fullMethodName)) {
		return "", "", "", errors.New("invalid full method name")
	}

	parts := strings.Split(fullMethodName, "/")
	methodName = parts[2]

	parts = strings.Split(parts[1], ".")
	serviceName = parts[len(parts)-1]
	packageName = strings.Join(parts[:len(parts)-1], ".")

	return packageName, serviceName, methodName, nil
}

// DisableEverythingUnaryServerInterceptor makes all unary RPCs return UNAVAILABLE.
// This is the nuclear option to stop all client requests and should be used sparingly.
func DisableEverythingUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Allow health checks to pass so we don't churn servers
		if IsHealthCheckEndpoint(info.FullMethod) {
			return handler(ctx, req)
		}

		return nil, status.Error(codes.Unavailable, "temporarily unavailable")
	}
}

// DisableEverythingStreamServerInterceptor makes all streaming RPCs return UNAVAILABLE.
// This is the nuclear option to stop all client requests and should be used sparingly.
func DisableEverythingStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return status.Error(codes.Unavailable, "temporarily unavailable")
	}
}

// IsHealthCheckEndpoint returns whether a method is the health check endpoint
func IsHealthCheckEndpoint(methodName string) bool {
	return methodName == healthCheckEndpoint
}
