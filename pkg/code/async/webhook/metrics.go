package async_webhook

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

const (
	webhookCountEventName = "WebhookCountPollingCheck"
	webhookCallsEventName = "WebhookCallsPollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			p.recordWebhookCountEvent(ctx, webhook.StatePending)
			p.recordWebhookCallsEvents(ctx)

			delay = time.Second - time.Since(start)
		}
	}
}

func (p *service) recordWebhookCountEvent(ctx context.Context, state webhook.State) {
	count, err := p.data.CountWebhookByState(ctx, state)
	if err != nil {
		return
	}

	metrics.RecordEvent(ctx, webhookCountEventName, map[string]interface{}{
		"count": count,
		"state": state.String(),
	})
}

func (p *service) recordWebhookCallsEvents(ctx context.Context) {
	p.metricsMu.Lock()
	successfulCalls := p.successfulWebhooks
	failedCalls := p.failedWebhooks
	p.successfulWebhooks = 0
	p.failedWebhooks = 0
	p.metricsMu.Unlock()

	metrics.RecordEvent(ctx, webhookCallsEventName, map[string]interface{}{
		"successes": successfulCalls,
		"failures":  failedCalls,
	})
}
