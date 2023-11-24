package validation

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Validator interface {
	Validate() error
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that validates
// inbound and outbound messages. If an inbound message is invalid, a
// codes.InvalidArgument is returned. If an outbound message is invalid, a
// codes.Internal is returned.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	log := logrus.StandardLogger().WithField("type", "protobuf/validation/interceptor")

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if v, ok := req.(Validator); ok {
			if err := v.Validate(); err != nil {
				// We use a debug level here because it is outside of 'our' control.
				log.WithError(err).Debug("dropping invalid request")
				return nil, status.Errorf(codes.InvalidArgument, err.Error())
			}
		}

		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}

		if v, ok := resp.(Validator); ok {
			if err := v.Validate(); err != nil {
				// We warn here because this indicates a problem with 'our' service.
				log.WithError(err).Warn("dropping invalid response")
				return nil, status.Errorf(codes.Internal, err.Error())
			}
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that validates
// inbound and outbound messages. If an inbound message is invalid, a
// codes.InvalidArgument is returned. If an outbound message is invalid, a
// codes.Internal is returned.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &serverStreamWrapper{logrus.StandardLogger().WithField("type", "protobuf/validation/interceptor"), ss})
	}
}

type serverStreamWrapper struct {
	log *logrus.Entry

	grpc.ServerStream
}

func (s *serverStreamWrapper) RecvMsg(req interface{}) error {
	if err := s.ServerStream.RecvMsg(req); err != nil {
		return err
	}

	if v, ok := req.(Validator); ok {
		if err := v.Validate(); err != nil {
			s.log.WithError(err).Debug("dropping invalid request")
			return status.Errorf(codes.InvalidArgument, err.Error())
		}
	}

	return nil
}

func (s *serverStreamWrapper) SendMsg(res interface{}) error {
	if v, ok := res.(Validator); ok {
		if err := v.Validate(); err != nil {
			s.log.WithError(err).Warn("dropping invalid response")
			return status.Errorf(codes.Internal, err.Error())
		}
	}

	return s.ServerStream.SendMsg(res)
}
