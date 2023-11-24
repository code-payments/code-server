package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"
)

func TestGetIPAddr(t *testing.T) {
	tt := map[string]struct {
		ctx        context.Context
		hasIP      bool
		expectedIP string
	}{
		"emptyContext": {
			ctx:   context.Background(),
			hasIP: false,
		},
		"contextWithoutClientIP": {
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.MD{"key": []string{"value"}},
			),
			hasIP: false,
		},
		"contextWithClientIP": {
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.MD{clientIPHeader: []string{"127.0.0.1"}},
			),
			hasIP:      true,
			expectedIP: "127.0.0.1",
		},
	}

	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			actual, err := GetIPAddr(tc.ctx)
			assert.Equal(t, tc.hasIP, err == nil)
			assert.Equal(t, tc.expectedIP, actual)
		})
	}
}
