package validation

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientInterceptor returns a grpc.UnaryClientInterceptor that validates
// inbound and outbound messages. If a service request is invalid, a
// codes.InvalidArgument is returned. If a service response is invalid, a
// codes.Internal is returned.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	log := logrus.StandardLogger().WithField("type", "protobuf/validation/interceptor")

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Validate request
		if v, ok := req.(Validator); ok {
			if err := v.Validate(); err != nil {
				// Log warn since the caller is at fault.
				log.WithError(err).Warn("dropping invalid request")
				return status.Errorf(codes.InvalidArgument, err.Error())
			}
		}

		// Do service call
		if err := invoker(ctx, method, req, reply, cc, opts...); err != nil {
			return err
		}

		// Validate service response
		if v, ok := reply.(Validator); ok {
			if err := v.Validate(); err != nil {
				// Just log debug here since the outbound service is mis-behaving.
				log.WithError(err).Debug("dropping invalid response")
				return status.Errorf(codes.Internal, err.Error())
			}
		}
		return nil
	}
}

// StreamClientInterceptor returns a grpc.StreamClientInterceptor that validates
// inbound and outbound messages. If any streamed service request is invalid, a
// codes.InvalidArgument is returned. If any streamed service response is invalid, a
// codes.Internal is returned.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	log := logrus.StandardLogger().WithField("type", "protobuf/validation/interceptor")

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		return &clientStreamWrapper{
			log:          log,
			ClientStream: clientStream,
		}, nil
	}
}

type clientStreamWrapper struct {
	log *logrus.Entry

	grpc.ClientStream
}

func (c *clientStreamWrapper) SendMsg(req interface{}) error {
	// Validate request
	if v, ok := req.(Validator); ok {
		if err := v.Validate(); err != nil {
			// Log warn since the caller is at fault.
			c.log.WithError(err).Warn("dropping invalid request")
			return status.Errorf(codes.InvalidArgument, err.Error())
		}
	}

	return c.ClientStream.SendMsg(req)
}

func (c *clientStreamWrapper) RecvMsg(res interface{}) error {
	if err := c.ClientStream.RecvMsg(res); err != nil {
		return err
	}

	// Validate service response
	if v, ok := res.(Validator); ok {
		if err := v.Validate(); err != nil {
			// Just log debug here since the outbound service is mis-behaving.
			c.log.WithError(err).Debug("dropping invalid response")
			return status.Errorf(codes.Internal, err.Error())
		}
	}
	return nil
}
