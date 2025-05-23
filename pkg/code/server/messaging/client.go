package messaging

import (
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/code-server/pkg/grpc/headers"
	"github.com/code-payments/code-server/pkg/grpc/protobuf/validation"
)

// todo: Generic utility for handling gRPC connections like this

var (
	internalMessagingClientConnsMu sync.RWMutex
	internalMessagingClientConns   map[string]*grpc.ClientConn
)

func init() {
	internalMessagingClientConns = make(map[string]*grpc.ClientConn)

	go periodicallyCleanupConns()
}

// todo: we can cache and reuse clients
func getInternalMessagingClient(address string) (messagingpb.MessagingClient, error) {
	internalMessagingClientConnsMu.RLock()
	existing, ok := internalMessagingClientConns[address]
	if ok {
		internalMessagingClientConnsMu.RUnlock()
		return messagingpb.NewMessagingClient(existing), nil
	}
	internalMessagingClientConnsMu.RUnlock()

	internalMessagingClientConnsMu.Lock()
	defer internalMessagingClientConnsMu.Unlock()

	existing, ok = internalMessagingClientConns[address]
	if ok {
		return messagingpb.NewMessagingClient(existing), nil
	}

	conn, err := grpc.NewClient(
		address,

		grpc.WithTransportCredentials(insecure.NewCredentials()),

		grpc.WithUnaryInterceptor(validation.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(headers.UnaryClientInterceptor()),

		grpc.WithStreamInterceptor(validation.StreamClientInterceptor()),
		grpc.WithStreamInterceptor(headers.StreamClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	internalMessagingClientConns[address] = conn
	return messagingpb.NewMessagingClient(conn), nil
}

func periodicallyCleanupConns() {
	for {
		time.Sleep(time.Minute)

		internalMessagingClientConnsMu.Lock()

		for target, conn := range internalMessagingClientConns {
			state := conn.GetState()
			switch state {
			case connectivity.TransientFailure, connectivity.Shutdown:
				conn.Close()
				delete(internalMessagingClientConns, target)
			}
		}

		internalMessagingClientConnsMu.Unlock()
	}
}
