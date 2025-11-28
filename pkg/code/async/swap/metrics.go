package async_swap

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	swapCountEventName     = "SwapCountPollingCheck"
	swapFinalizedEventName = "SwapFinalized"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			for _, state := range []swap.State{
				swap.StateCreated,
				swap.StateFunding,
				swap.StateFunded,
				swap.StateSubmitting,
				swap.StateFailed,
				swap.StateCancelling,
			} {
				count, err := p.data.GetSwapCountByState(ctx, state)
				if err != nil {
					continue
				}
				recordSwapCountEvent(ctx, state, count)
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func recordSwapCountEvent(ctx context.Context, state swap.State, count uint64) {
	metrics.RecordEvent(ctx, swapCountEventName, map[string]interface{}{
		"count": count,
		"state": state.String(),
	})
}

func recordSwapFinalizedEvent(ctx context.Context, swapRecord *swap.Record, quarksBought uint64) {
	metrics.RecordEvent(ctx, swapFinalizedEventName, map[string]interface{}{
		"id":            swapRecord.Id,
		"from_mint":     swapRecord.FromMint,
		"to_mint":       swapRecord.ToMint,
		"quarks_sold":   swapRecord.Amount,
		"quarks_bought": quarksBought,
	})
}
