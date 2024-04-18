package metrics

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/newrelic/go-agent/v3/newrelic"
	grpc_core "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/code-payments/code-server/pkg/grpc"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

type statusCodeHandler func(*newrelic.Transaction, *status.Status)
type resultCodeHandler func(*newrelic.Transaction, string)

const (
	grpcRequestPackageAttributeKey = "grpc.request.package"
	grpcRequestServiceAttributeKey = "grpc.request.service"
	grpcRequestMethodAttributeKey  = "grpc.request.method"

	grpcResponseStatusCodeAttributeKey      = "grpc.response.statusCode"
	grpcResponseStatusMessageAttributeKey   = "grpc.response.statusMessage"
	grpcResponseStatusCodeLevelAttributeKey = "grpc.response.statusCodeLevel"

	resultCodeAttributeKey      = "code.response.resultCode"
	resultCodeLevelAttributeKey = "code.response.resultCodeLevel"

	clientUserAgentAttributeKey = "grpc.client.userAgent"

	infoLevel    = "info"
	warningLevel = "warning"
	errorLevel   = "error"
)

var (
	statusCodeHandlers = map[codes.Code]statusCodeHandler{
		codes.OK:              infoStatusCodeHandler,
		codes.AlreadyExists:   infoStatusCodeHandler,
		codes.Canceled:        infoStatusCodeHandler,
		codes.InvalidArgument: infoStatusCodeHandler,
		codes.NotFound:        infoStatusCodeHandler,
		codes.Unauthenticated: infoStatusCodeHandler,

		codes.Aborted:            warningStatusCodeHandler,
		codes.DeadlineExceeded:   warningStatusCodeHandler,
		codes.FailedPrecondition: warningStatusCodeHandler,
		codes.OutOfRange:         warningStatusCodeHandler,
		codes.PermissionDenied:   warningStatusCodeHandler,
		codes.ResourceExhausted:  warningStatusCodeHandler,
		codes.Unavailable:        warningStatusCodeHandler,

		codes.DataLoss:      errorStatusCodeHandler,
		codes.Unknown:       errorStatusCodeHandler,
		codes.Internal:      errorStatusCodeHandler,
		codes.Unimplemented: errorStatusCodeHandler,
	}
	defaultStatusCodeHandler = errorStatusCodeHandler

	resultCodeHandlers = map[string]resultCodeHandler{
		"OK":                    infoResultCodeHandler,
		"NOOP":                  infoResultCodeHandler,
		"NOT_FOUND":             infoResultCodeHandler,
		"ACTION_NOT_FOUND":      infoResultCodeHandler,
		"INTENT_NOT_FOUND":      infoResultCodeHandler,
		"NO_VERIFICATION":       infoResultCodeHandler,
		"EXISTS":                infoResultCodeHandler,
		"ALREADY_INVITED":       infoResultCodeHandler,
		"NOT_INVITED":           infoResultCodeHandler,
		"SENDER_NOT_INVITED":    infoResultCodeHandler,
		"INVITE_COUNT_EXCEEDED": infoResultCodeHandler,

		"DENIED":                        warningResultCodeHandler,
		"INVALID_ACTION":                warningResultCodeHandler,
		"INVALID_CODE":                  warningResultCodeHandler,
		"INVALID_INTENT":                warningResultCodeHandler,
		"INVALID_PHONE_NUMBER":          warningResultCodeHandler,
		"INVALID_PUSH_TOKEN":            warningResultCodeHandler,
		"INVALID_RECEIVER_PHONE_NUMBER": warningResultCodeHandler,
		"INVALID_TOKEN":                 warningResultCodeHandler,
		"LANG_UNAVAILABLE":              warningResultCodeHandler,
		"RATE_LIMITED":                  warningResultCodeHandler,
		"SIGNATURE_ERROR":               warningResultCodeHandler,
	}
	defaultResultCodeHandler = errorResultCodeHandler
)

func infoStatusCodeHandler(m *newrelic.Transaction, s *status.Status) {
	m.SetWebResponse(nil).WriteHeader(int(codes.OK))
	m.AddAttribute(grpcResponseStatusCodeAttributeKey, s.Code().String())
	m.AddAttribute(grpcResponseStatusMessageAttributeKey, s.Message())
	m.AddAttribute(grpcResponseStatusCodeLevelAttributeKey, infoLevel)
}

func warningStatusCodeHandler(m *newrelic.Transaction, s *status.Status) {
	m.SetWebResponse(nil).WriteHeader(int(codes.OK))
	m.AddAttribute(grpcResponseStatusCodeAttributeKey, s.Code().String())
	m.AddAttribute(grpcResponseStatusMessageAttributeKey, s.Message())
	m.AddAttribute(grpcResponseStatusCodeLevelAttributeKey, warningLevel)
}

func errorStatusCodeHandler(m *newrelic.Transaction, s *status.Status) {
	m.SetWebResponse(nil).WriteHeader(int(codes.OK))
	m.AddAttribute(grpcResponseStatusCodeAttributeKey, s.Code().String())
	m.AddAttribute(grpcResponseStatusMessageAttributeKey, s.Message())
	m.AddAttribute(grpcResponseStatusCodeLevelAttributeKey, errorLevel)
	m.NoticeError(&newrelic.Error{
		Message: s.Message(),
		Class:   "gRPC Status: " + s.Code().String(),
	})
}

func infoResultCodeHandler(m *newrelic.Transaction, resultCode string) {
	m.AddAttribute(resultCodeAttributeKey, resultCode)
	m.AddAttribute(resultCodeLevelAttributeKey, infoLevel)
}

func warningResultCodeHandler(m *newrelic.Transaction, resultCode string) {
	m.AddAttribute(resultCodeAttributeKey, resultCode)
	m.AddAttribute(resultCodeLevelAttributeKey, warningLevel)
}

func errorResultCodeHandler(m *newrelic.Transaction, resultCode string) {
	m.AddAttribute(resultCodeAttributeKey, resultCode)
	m.AddAttribute(resultCodeLevelAttributeKey, errorLevel)
	m.NoticeError(&newrelic.Error{
		Class: "Code RPC Result: " + resultCode,
	})
}

// CustomNewRelicUnaryServerInterceptor is a custom implementation of the New
// Relic unary interceptor.
func CustomNewRelicUnaryServerInterceptor(app *newrelic.Application) grpc_core.UnaryServerInterceptor {
	if app == nil {
		return func(ctx context.Context, req interface{}, info *grpc_core.UnaryServerInfo, handler grpc_core.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(ctx context.Context, req interface{}, info *grpc_core.UnaryServerInfo, handler grpc_core.UnaryHandler) (interface{}, error) {
		// Inject the application to allow for any custom metrics, events, etc
		// in downstream code.
		ctx = context.WithValue(ctx, metrics.NewRelicContextKey{}, app)

		m := startTransaction(ctx, app, info.FullMethod)
		defer m.End()

		ctx = newrelic.NewContext(ctx, m)

		includeParsedFullMethodName(m, info.FullMethod)
		includeClientMetadata(ctx, m)

		resp, err := handler(ctx, req)
		includeGRPCStatusCode(m, err)
		if err != nil {
			return nil, err
		}

		reflected := resp.(proto.Message).ProtoReflect()
		includeCodeResultCodeForUnaryCall(m, reflected)

		return resp, nil
	}
}

func CustomNewRelicStreamServerInterceptor(app *newrelic.Application) grpc_core.StreamServerInterceptor {
	if app == nil {
		return func(srv interface{}, ss grpc_core.ServerStream, info *grpc_core.StreamServerInfo, handler grpc_core.StreamHandler) error {
			return handler(srv, ss)
		}
	}

	return func(srv interface{}, ss grpc_core.ServerStream, info *grpc_core.StreamServerInfo, handler grpc_core.StreamHandler) error {
		// Inject the application to allow for any custom metrics, events, etc
		// in downstream code.
		ctx := context.WithValue(ss.Context(), metrics.NewRelicContextKey{}, app)

		m := startTransaction(ctx, app, info.FullMethod)
		defer m.End()

		ctx = newrelic.NewContext(ctx, m)

		includeParsedFullMethodName(m, info.FullMethod)
		includeClientMetadata(ctx, m)

		err := handler(srv, newWrappedStream(ctx, m, ss))
		includeGRPCStatusCode(m, err)
		return err
	}
}

type wrappedStream struct {
	ctx context.Context
	txn *newrelic.Transaction
	grpc_core.ServerStream
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

func (w *wrappedStream) SendMsg(m interface{}) error {
	reflected := m.(proto.Message).ProtoReflect()
	includeCodeResultCodeForServerStreamCall(w.txn, reflected)
	return w.ServerStream.SendMsg(m)
}

func newWrappedStream(ctx context.Context, m *newrelic.Transaction, wrapped grpc_core.ServerStream) grpc_core.ServerStream {
	return &wrappedStream{ctx, m, wrapped}
}

func startTransaction(ctx context.Context, app *newrelic.Application, fullMethod string) *newrelic.Transaction {
	method := strings.TrimPrefix(fullMethod, "/")

	// todo: we may not want to include all headers, especially if they contain
	//       sensitive information
	var hdrs http.Header
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		hdrs = make(http.Header, len(md))
		for k, vs := range md {
			for _, v := range vs {
				hdrs.Add(k, v)
			}
		}
	}

	target := hdrs.Get(":authority")
	url := getURL(method, target)

	webReq := newrelic.WebRequest{
		Header:    hdrs,
		URL:       url,
		Method:    method,
		Transport: newrelic.TransportHTTP,
	}
	txn := app.StartTransaction(method)
	txn.SetWebRequest(webReq)

	return txn
}

func getURL(method, target string) *url.URL {
	var host string
	// target can be anything from
	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	// see https://godoc.org/google.golang.org/grpc#DialContext
	if strings.HasPrefix(target, "unix:") {
		host = "localhost"
	} else {
		host = strings.TrimPrefix(target, "dns:///")
	}
	return &url.URL{
		Scheme: "grpc",
		Host:   host,
		Path:   method,
	}
}

func includeGRPCStatusCode(m *newrelic.Transaction, err error) {
	grpcStatus := status.Convert(err)
	handler, ok := statusCodeHandlers[grpcStatus.Code()]
	if !ok {
		handler = defaultStatusCodeHandler
	}
	handler(m, grpcStatus)
}

func includeCodeResultCodeForUnaryCall(m *newrelic.Transaction, reflected protoreflect.Message) {
	// Check whether the response message has an enum called Result
	resultEnumDescriptor := reflected.Descriptor().Enums().ByName("Result")
	if resultEnumDescriptor == nil {
		return
	}

	// Check whether the response message has a field called result
	resultFieldDescriptor := reflected.Descriptor().Fields().ByName("result")
	if resultFieldDescriptor == nil {
		return
	}

	// This is the only sketchy part of the implementation. It'll panic if
	// the field isn't an enum. It seems unlikely, because we've already
	// determined an enum named Result exists, so we'd expect a reasonable
	// field name of result.
	resultEnumNumber := reflected.Get(resultFieldDescriptor).Enum()

	resultEnum := resultEnumDescriptor.Values().ByNumber(resultEnumNumber)
	if resultEnum == nil {
		return
	}

	// Augment the transaction
	resultCode := strings.ToUpper(string(resultEnum.Name()))
	handler, ok := resultCodeHandlers[resultCode]
	if !ok {
		defaultResultCodeHandler(m, resultCode)
	} else {
		handler(m, resultCode)
	}
}

// todo: currently assumes SubmitIntent standards
func includeCodeResultCodeForServerStreamCall(m *newrelic.Transaction, reflected protoreflect.Message) {
	// Check whether the response message has a field set called success or error
	var respMessage protoreflect.Message
	resultMessageFieldDescriptor := reflected.Descriptor().Fields().ByName("success")
	if resultMessageFieldDescriptor != nil {
		respMessage = reflected.Get(resultMessageFieldDescriptor).Message()
	}
	if respMessage == nil || !respMessage.IsValid() {
		resultMessageFieldDescriptor := reflected.Descriptor().Fields().ByName("error")
		if resultMessageFieldDescriptor != nil {
			respMessage = reflected.Get(resultMessageFieldDescriptor).Message()
		}
	}
	if respMessage == nil || !respMessage.IsValid() {
		return
	}

	// Check whether the response message has an enum called Code
	resultEnumDescriptor := respMessage.Descriptor().Enums().ByName("Code")
	if resultEnumDescriptor == nil {
		return
	}

	// Check whether the response message has a field called code
	resultEnumFieldDescriptor := respMessage.Descriptor().Fields().ByName("code")
	if resultEnumFieldDescriptor == nil {
		return
	}

	// This is the only sketchy part of the implementation. It'll panic if
	// the field isn't an enum. It seems unlikely, because we've already
	// determined an enum named Result exists, so we'd expect a reasonable
	// field name of result.
	resultEnumNumber := respMessage.Get(resultEnumFieldDescriptor).Enum()

	resultEnum := resultEnumDescriptor.Values().ByNumber(resultEnumNumber)
	if resultEnum == nil {
		return
	}

	// Augment the transaction
	resultCode := strings.ToUpper(string(resultEnum.Name()))
	handler, ok := resultCodeHandlers[resultCode]
	if !ok {
		defaultResultCodeHandler(m, resultCode)
	} else {
		handler(m, resultCode)
	}
}

func includeParsedFullMethodName(m *newrelic.Transaction, fullMethodName string) {
	packageName, serviceName, methodName, err := grpc.ParseFullMethodName(fullMethodName)
	if err != nil {
		return
	}

	m.AddAttribute(grpcRequestPackageAttributeKey, packageName)
	m.AddAttribute(grpcRequestServiceAttributeKey, serviceName)
	m.AddAttribute(grpcRequestMethodAttributeKey, methodName)
}

func includeClientMetadata(ctx context.Context, m *newrelic.Transaction) {
	userAgent, err := client.GetUserAgent(ctx)
	if err == nil {
		m.AddAttribute(clientUserAgentAttributeKey, userAgent.String())
	}
}
