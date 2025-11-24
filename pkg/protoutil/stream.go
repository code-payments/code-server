package protoutil

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Ptr[T any] interface {
	proto.Message
	*T
}

func BoundedReceive[Req any](
	ctx context.Context,
	stream grpc.ServerStream,
	timeout time.Duration,
) (*Req, error) {
	var err error
	var req = new(Req)
	doneCh := make(chan struct{})

	go func() {
		err = stream.RecvMsg(req)
		close(doneCh)
	}()

	select {
	case <-doneCh:
		return req, err
	case <-ctx.Done():
		return req, status.Error(codes.Canceled, "")
	case <-time.After(timeout):
		return req, status.Error(codes.DeadlineExceeded, "timeout receiving message")
	}
}
