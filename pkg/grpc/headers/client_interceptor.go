package headers

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor sends all the headers in the current context to server
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	log := logrus.StandardLogger().WithField("type", "headers/interceptor")

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = setAllHeaders(ctx, log)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor sends all the headers in the current context to a server stream
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	log := logrus.StandardLogger().WithField("type", "headers/interceptor")

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = setAllHeaders(ctx, log)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// setAllHeaders Take all the headers currently in the context, except for the incoming Type,
// and put them into the metadata to be passed on to the next service
func setAllHeaders(ctx context.Context, log *logrus.Entry) context.Context {
	allHeader := Headers{}

	if rootHeader, ok := (ctx).Value(rootHeaderKey).(Headers); ok {
		log.Trace("Found and transporting root header")
		allHeader.merge(rootHeader)
	}
	if propagatingHeader, ok := (ctx).Value(propagatingHeaderKey).(Headers); ok {
		log.Trace("Found and transporting propagating header")
		allHeader.merge(propagatingHeader)
	}
	if outboundHeader, ok := (ctx).Value(outboundBinaryHeaderKey).(Headers); ok {
		log.Trace("Found and transporting default header")
		allHeader.merge(outboundHeader)
	}
	if asciiHeader, ok := (ctx).Value(asciiHeaderKey).(Headers); ok {
		log.Trace("Found and transporting ascii header")
		allHeader.merge(asciiHeader)
	}

	for k, v := range allHeader {
		switch strings.ToLower(k) {
		case "content-type", "user-agent", ":authority":
			continue
		}
		if val, ok := v.([]byte); ok {
			ctx = metadata.AppendToOutgoingContext(ctx, k, string(val))
		} else if val, ok := v.(string); ok {
			ctx = metadata.AppendToOutgoingContext(ctx, k, val)
		} else {
			log.WithField("name", k).Warnf("Unexpected header value type: %T", v)
		}
	}

	return ctx
}
