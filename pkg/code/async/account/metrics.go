package async_account

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	giftCardWorkerEventName  = "GiftCardWorkerPollingCheck"
	swapRetryWorkerEventName = "SwapRetryWorkerPollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			p.recordBackupQueueStatusPollingEvent(ctx)

			delay = time.Second - time.Since(start)
		}
	}
}

func (p *service) recordBackupQueueStatusPollingEvent(ctx context.Context) {
	count, err := p.data.GetAccountInfoCountRequiringAutoReturnCheck(ctx)
	if err == nil {
		metrics.RecordEvent(ctx, giftCardWorkerEventName, map[string]interface{}{
			"queue_size": count,
		})
	}

	count, err = p.data.GetAccountInfoCountRequiringSwapRetry(ctx)
	if err == nil {
		metrics.RecordEvent(ctx, swapRetryWorkerEventName, map[string]interface{}{
			"queue_size": count,
		})
	}
}
