package headers

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that takes all the appropriate
// headerPrefixes from the metadata and puts it into the context
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	log := logrus.StandardLogger().WithField("type", "headers/interceptor")
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = getAllHeaders(ctx, log)
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that takes all the appropriate
// headerPrefixes from the metadata and puts it into the context of the streamWrapper
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		log := logrus.StandardLogger().WithField("type", "headers/interceptor")

		ctx := getAllHeaders(ss.Context(), log)
		return handler(srv, &streamWrapper{log: log, ServerStream: ss, ctx: ctx})
	}
}

type streamWrapper struct {
	log *logrus.Entry
	grpc.ServerStream
	ctx context.Context
}

// RecvMsg is needed to conform to the grpc.ServerStream interface
func (s *streamWrapper) RecvMsg(msg interface{}) error {
	return s.ServerStream.RecvMsg(msg)
}

// SendMsg is needed to conform to the grpc.ServerStream interface
func (s *streamWrapper) SendMsg(msg interface{}) error {
	return s.ServerStream.SendMsg(msg)
}

// Context overriding so we can send out custom context with all the Header information
func (s *streamWrapper) Context() context.Context {
	return s.ctx
}

// getAllHeaders retrieve all the headerPrefixes from the contexts metadata, and puts them into the
// context to be easily accessible
func getAllHeaders(ctx context.Context, log *logrus.Entry) context.Context {
	var rootHeader = Headers{}
	var propagatingHeader = Headers{}
	var incomingHeader = Headers{}
	var asciiHeader = Headers{}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for key := range md {
			// Shouldn't ever happen, but check just in case
			if len(md[key]) < 1 {
				log.Infof("key %s had no data in headers", key)
				continue
			}

			if strings.HasPrefix(key, Root.prefix()) {
				rootHeader[key] = []byte(md[key][0])
			} else if strings.HasPrefix(key, Propagating.prefix()) {
				propagatingHeader[key] = []byte(md[key][0])
			} else {
				// Note root and propagating headers can only be binary. Only the default header has a chance to be an ascii header
				if strings.HasSuffix(key, "-bin") {
					incomingHeader[key] = []byte(md[key][0])
				} else {
					asciiHeader[key] = md[key][0]
				}
			}
		}
	}
	ctx = context.WithValue(ctx, rootHeaderKey, rootHeader)
	ctx = context.WithValue(ctx, propagatingHeaderKey, propagatingHeader)
	ctx = context.WithValue(ctx, inboundBinaryHeaderKey, incomingHeader)
	ctx = context.WithValue(ctx, asciiHeaderKey, asciiHeader)
	ctx = context.WithValue(ctx, outboundBinaryHeaderKey, Headers{})
	return ctx
}
