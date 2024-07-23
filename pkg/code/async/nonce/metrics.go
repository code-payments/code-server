package async_nonce

import (
	"context"
	"fmt"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	nonceCountCheckEventName = "NonceCountPollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	cvmPublicKey := p.conf.cvmPublicKey.Get(ctx)

	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			// todo: optimize number of queries needed per polling check
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
					count, err := p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, useCase)
					if err != nil {
						continue
					}
					recordNonceCountEvent(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, useCase, count)

					count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentCvm, cvmPublicKey, state, useCase)
					if err != nil {
						continue
					}
					recordNonceCountEvent(ctx, nonce.EnvironmentCvm, cvmPublicKey, state, useCase, count)
				}
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func recordNonceCountEvent(ctx context.Context, env nonce.Environment, instance string, state nonce.State, useCase nonce.Purpose, count uint64) {
	metrics.RecordEvent(ctx, nonceCountCheckEventName, map[string]interface{}{
		"pool":     fmt.Sprintf("%s:%s", env.String(), instance),
		"use_case": useCase.String(),
		"state":    state.String(),
		"count":    count,
	})
}
