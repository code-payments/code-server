package async_sequencer

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

const (
	fulfillmentCountEventName                  = "FulfillmentCountPollingCheck"
	subsidizerBalanceEventName                 = "SubsidizerBalancePollingCheck"
	temporaryPrivateTransferScheduledEventName = "TemporaryPrivateTransferScheduled"
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

			lamports, err := common.GetCurrentSubsidizerBalance(ctx, p.data)
			if err == nil {
				recordSubsidizerBalanceEvent(ctx, lamports)
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

func recordSubsidizerBalanceEvent(ctx context.Context, lamports uint64) {
	metrics.RecordEvent(ctx, subsidizerBalanceEventName, map[string]interface{}{
		"lamports": lamports,
	})
}

func recordTemporaryPrivateTransferScheduledEvent(ctx context.Context, fulfillmentRecord *fulfillment.Record) {
	metrics.RecordEvent(ctx, temporaryPrivateTransferScheduledEventName, map[string]interface{}{
		"signature": *fulfillmentRecord.Signature,
	})
}
