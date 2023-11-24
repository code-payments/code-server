package async

import (
	"context"
	"time"
)

type Service interface {
	Start(ctx context.Context, interval time.Duration) error
}
