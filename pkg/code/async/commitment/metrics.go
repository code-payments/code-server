package async_commitment

import (
	"context"
	"fmt"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

const (
	commitmentCountMetricName = "Commitment/%s_count"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			for _, state := range []commitment.State{
				commitment.StateUnknown,
				commitment.StatePayingDestination,
				commitment.StateReadyToOpen,
				commitment.StateOpening,
				commitment.StateOpen,
				commitment.StateClosing,
				commitment.StateClosed,
			} {
				count, err := p.data.CountCommitmentsByState(ctx, state)
				if err != nil {
					continue
				}
				recordCommitmentCountMetric(ctx, state, count)
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func recordCommitmentCountMetric(ctx context.Context, state commitment.State, count uint64) {
	metricName := fmt.Sprintf(commitmentCountMetricName, state.String())
	metrics.RecordCount(ctx, metricName, count)
}
