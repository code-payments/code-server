package messaging

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/code-server/pkg/grpc/headers"
	"github.com/code-payments/code-server/pkg/grpc/protobuf/validation"
)

// todo: we can cache and reuse clients
func getInternalMessagingClient(target string) (messagingpb.MessagingClient, func() error, error) {
	conn, err := grpc.Dial(
		target,

		grpc.WithTransportCredentials(insecure.NewCredentials()),

		grpc.WithUnaryInterceptor(validation.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(headers.UnaryClientInterceptor()),

		grpc.WithStreamInterceptor(validation.StreamClientInterceptor()),
		grpc.WithStreamInterceptor(headers.StreamClientInterceptor()),
	)
	if err != nil {
		return nil, nil, err
	}

	return messagingpb.NewMessagingClient(conn), conn.Close, nil
}
