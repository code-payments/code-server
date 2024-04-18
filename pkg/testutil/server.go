package testutil

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/code-payments/code-server/pkg/grpc/headers"
	"github.com/code-payments/code-server/pkg/grpc/protobuf/validation"
	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

// Server provides a local gRPC server with basic interceptors that
// can be used for testing with no external dependencies.
type Server struct {
	closeFunc sync.Once

	sync.Mutex
	serv       bool
	listener   net.Listener
	grpcServer *grpc.Server
	clientConn *grpc.ClientConn
}

// NewServer creates a new Server.
func NewServer(opts ...ServerOption) (*grpc.ClientConn, *Server, error) {
	port, err := netutil.GetAvailablePortForAddress("localhost")
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to find free port")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to start listener")
	}

	o := serverOpts{
		unaryServerInterceptors: []grpc.UnaryServerInterceptor{
			headers.UnaryServerInterceptor(),
			validation.UnaryServerInterceptor(),
		},
		streamServerInterceptors: []grpc.StreamServerInterceptor{
			headers.StreamServerInterceptor(),
			validation.StreamServerInterceptor(),
		},
		unaryClientInterceptors: []grpc.UnaryClientInterceptor{
			validation.UnaryClientInterceptor(),
		},
		streamClientInterceptors: []grpc.StreamClientInterceptor{
			validation.StreamClientInterceptor(),
		},
	}

	for _, opt := range opts {
		opt(&o)
	}

	// note: this is safe since we don't specify grpc.WithBlock()
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(o.unaryClientInterceptors...)),
		grpc.WithStreamInterceptor(grpc_middleware.ChainStreamClient(o.streamClientInterceptors...)),
	)

	if err != nil {
		listener.Close()
		return nil, nil, errors.Wrapf(err, "failed to create grpc.ClientConn")
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(o.unaryServerInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(o.streamServerInterceptors...)),
	)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	return conn, &Server{
		listener:   listener,
		grpcServer: grpcServer,
		clientConn: conn,
	}, nil
}

// RegisterService registers a gRPC service with Server.
func (s *Server) RegisterService(registerFunc func(s *grpc.Server)) {
	registerFunc(s.grpcServer)
}

// Serve asynchronously starts the server, provided it has not been previously
// started or stopped. Callers should use stopFunc to stop the server in order
// to cleanup the underlying resources.
func (s *Server) Serve() (stopFunc func(), err error) {
	// Start the server
	err = func() error {
		s.Lock()
		defer s.Unlock()

		if s.serv {
			return nil
		}

		if s.grpcServer == nil {
			return errors.Errorf("testserver already stopped")
		}

		stopFunc = func() {
			s.closeFunc.Do(func() {
				s.Lock()
				defer s.Unlock()

				s.grpcServer.Stop()
				s.listener.Close()
				s.grpcServer = nil
				s.listener = nil
			})
		}

		go func() {
			s.Lock()
			lis := s.listener
			serv := s.grpcServer
			s.Unlock()

			if lis == nil || serv == nil {
				return
			}

			err := serv.Serve(lis)
			logrus.
				StandardLogger().
				WithField("type", "testutil/server").
				WithError(err).
				Debug("stopped")
			stopFunc()
		}()

		s.serv = true
		return nil
	}()
	if err != nil {
		return nil, err
	}

	// Verify we can call a RPC
	_, err = retry.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		healthClient := grpc_health_v1.NewHealthClient(s.clientConn)
		_, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		return err
	}, retry.Limit(10), retry.Backoff(backoff.Constant(250*time.Millisecond), 250*time.Millisecond))
	if err != nil {
		return nil, errors.Wrap(err, "error executing sanity test rpc call")
	}

	return stopFunc, nil
}

type serverOpts struct {
	unaryClientInterceptors  []grpc.UnaryClientInterceptor
	streamClientInterceptors []grpc.StreamClientInterceptor

	unaryServerInterceptors  []grpc.UnaryServerInterceptor
	streamServerInterceptors []grpc.StreamServerInterceptor
}

// ServerOption configures the settings when creating a test server.
type ServerOption func(o *serverOpts)

// WithUnaryClientInterceptor adds a unary client interceptor to the test client.
func WithUnaryClientInterceptor(i grpc.UnaryClientInterceptor) ServerOption {
	return func(o *serverOpts) {
		o.unaryClientInterceptors = append(o.unaryClientInterceptors, i)
	}
}

// WithStreamClientInterceptor adds a stream client interceptor to the test client.
func WithStreamClientInterceptor(i grpc.StreamClientInterceptor) ServerOption {
	return func(o *serverOpts) {
		o.streamClientInterceptors = append(o.streamClientInterceptors, i)
	}
}

// WithUnaryServerInterceptor adds a unary server interceptor to the test client.
func WithUnaryServerInterceptor(i grpc.UnaryServerInterceptor) ServerOption {
	return func(o *serverOpts) {
		o.unaryServerInterceptors = append(o.unaryServerInterceptors, i)
	}
}

// WithStreamServerInterceptor adds a stream server interceptor to the test client.
func WithStreamServerInterceptor(i grpc.StreamServerInterceptor) ServerOption {
	return func(o *serverOpts) {
		o.streamServerInterceptors = append(o.streamServerInterceptors, i)
	}
}
