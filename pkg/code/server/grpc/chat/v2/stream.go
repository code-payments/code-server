package chat_v2

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
)

func boundedStreamChatEventsRecv(
	ctx context.Context,
	streamer chatpb.Chat_StreamChatEventsServer,
	timeout time.Duration,
) (req *chatpb.StreamChatEventsRequest, err error) {
	done := make(chan struct{})
	go func() {
		req, err = streamer.Recv()
		close(done)
	}()

	select {
	case <-done:
		return req, err
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, "")
	case <-time.After(timeout):
		return nil, status.Error(codes.DeadlineExceeded, "timed out receiving message")
	}
}
