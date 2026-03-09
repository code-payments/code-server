package async_sequencer

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	fulfillmentCountEventName  = "FulfillmentCountPollingCheck"
	subsidizerBalanceEventName = "SubsidizerBalancePollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			for _, state := range []fulfillment.State{
				fulfillment.StateUnknown,
				fulfillment.StatePending,
				fulfillment.StateFailed,
			} {
				countByType, err := p.data.GetFulfillmentCountForMetrics(ctx, state)
				if err != nil {
					continue
				}

				for fulfillmentType, count := range countByType {
					recordFulfillmentCountEvent(ctx, fulfillmentType, state, count)
				}
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func recordFulfillmentCountEvent(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, count uint64) {
	metrics.RecordEvent(ctx, fulfillmentCountEventName, map[string]interface{}{
		"count": count,
		"state": state.String(),
		"type":  fulfillmentType.String(),
	})
}
