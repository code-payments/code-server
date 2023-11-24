package client

import (
	"context"
	"errors"

	"google.golang.org/grpc/metadata"
)

const (
	clientIPHeader = "x-forwarded-for"
)

// GetIPAddr gets the client's IP address
func GetIPAddr(ctx context.Context) (string, error) {
	mtdt, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metdata in context")
	}

	ipHeaders := mtdt.Get(clientIPHeader)
	if len(ipHeaders) == 0 {
		return "", errors.New("x-forwarded-for header not set")
	}

	return ipHeaders[0], nil
}
