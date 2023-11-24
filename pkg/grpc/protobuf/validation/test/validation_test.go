package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/code-payments/code-server/pkg/testutil"
)

type service struct{}

func (s *service) Ping(_ context.Context, req *PingRequest) (*PingResponse, error) {
	return &PingResponse{Id: req.Id + 1}, nil
}

func (s *service) PingStream(stream MyService_PingStreamServer) error {
	var r *PingRequest
	var err error

	for r, err = stream.Recv(); err == nil; r, err = stream.Recv() {
		if err = stream.Send(&PingResponse{Id: r.Id + 1}); err != nil {
			break
		}
	}

	return err
}

func TestUnary(t *testing.T) {
	cc, serv, err := testutil.NewServer()
	require.NoError(t, err)

	serv.RegisterService(func(s *grpc.Server) {
		RegisterMyServiceServer(s, &service{})
	})

	stopFunc, err := serv.Serve()
	require.NoError(t, err)
	defer stopFunc()

	client := NewMyServiceClient(cc)

	resp, err := client.Ping(context.Background(), &PingRequest{
		Id: 1,
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, resp.Id)

	_, err = client.Ping(context.Background(), &PingRequest{})
	assert.EqualValues(t, codes.InvalidArgument, status.Code(err))

	// 99+1 will exceed the response limit, but _not_ the request limit
	_, err = client.Ping(context.Background(), &PingRequest{Id: 99})
	assert.EqualValues(t, codes.Internal, status.Code(err))
}

func TestStream(t *testing.T) {
	cc, serv, err := testutil.NewServer()
	require.NoError(t, err)

	serv.RegisterService(func(s *grpc.Server) {
		RegisterMyServiceServer(s, &service{})
	})

	stopFunc, err := serv.Serve()
	require.NoError(t, err)
	defer stopFunc()

	client := NewMyServiceClient(cc)
	stream, err := client.PingStream(context.Background())
	require.NoError(t, err)

	assert.NoError(t, stream.Send(&PingRequest{Id: 1}))
	resp, err := stream.Recv()
	assert.NoError(t, err)
	assert.EqualValues(t, 2, resp.Id)

	assert.Equal(t, codes.InvalidArgument, status.Code(stream.Send(&PingRequest{Id: 0})))

	stream, err = client.PingStream(context.Background())
	require.NoError(t, err)

	assert.NoError(t, stream.Send(&PingRequest{Id: 99}))
	_, err = stream.Recv()
	assert.Equal(t, codes.Internal, status.Code(err))
}
