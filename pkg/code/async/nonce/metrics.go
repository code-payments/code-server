package async_nonce

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
)

const (
	nonceCountMetricName     = "Nonce/%s_count"
	nonceCountCheckEventName = "NonceCountPollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			for _, useCase := range []nonce.Purpose{
				nonce.PurposeClientTransaction,
				nonce.PurposeInternalServerProcess,
				nonce.PurposeOnDemandTransaction,
			} {
				for _, state := range []nonce.State{
					nonce.StateUnknown,
					nonce.StateReleased,
					nonce.StateAvailable,
					nonce.StateReserved,
					nonce.StateInvalid,
				} {
					count, err := p.data.GetNonceCountByStateAndPurpose(ctx, state, useCase)
					if err != nil {
						continue
					}

					recordNonceCountEvent(ctx, state, useCase, count)
				}
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func recordNonceCountEvent(ctx context.Context, state nonce.State, useCase nonce.Purpose, count uint64) {
	metrics.RecordEvent(ctx, nonceCountCheckEventName, map[string]interface{}{
		"use_case": useCase.String(),
		"state":    state.String(),
		"count":    count,
	})
}
