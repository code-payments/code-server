package async_nonce

import (
	"context"
	"fmt"
	"time"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
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

			// todo: optimize number of queries needed per polling check
			for _, state := range []nonce.State{
				nonce.StateUnknown,
				nonce.StateReleased,
				nonce.StateAvailable,
				nonce.StateReserved,
				nonce.StateInvalid,
				nonce.StateClaimed,
			} {
				count, err := p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentCvm, common.CodeVmAccount.PublicKey().ToBase58(), state, nonce.PurposeClientTransaction)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentCvm, common.CodeVmAccount.PublicKey().ToBase58(), state, nonce.PurposeClientTransaction, count)

				count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentCvm, "Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta", state, nonce.PurposeClientTransaction)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentCvm, "Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta", state, nonce.PurposeClientTransaction, count)

				count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentCvm, "9Du5GuKYT21ydLQ9KzUTWWQ7NKdwoXB15y4ypNnnpbJa", state, nonce.PurposeClientTransaction)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentCvm, "9Du5GuKYT21ydLQ9KzUTWWQ7NKdwoXB15y4ypNnnpbJa", state, nonce.PurposeClientTransaction, count)

				count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentCvm, "5x9SP9a7dEGxK4xy8kurh8RC2fxvL1DSXhTCdcAMgpdb", state, nonce.PurposeClientTransaction)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentCvm, "5x9SP9a7dEGxK4xy8kurh8RC2fxvL1DSXhTCdcAMgpdb", state, nonce.PurposeClientTransaction, count)

				count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, nonce.PurposeOnDemandTransaction)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, nonce.PurposeOnDemandTransaction, count)

				count, err = p.data.GetNonceCountByStateAndPurpose(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, nonce.PurposeInternalServerProcess)
				if err != nil {
					continue
				}
				recordNonceCountEvent(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, nonce.PurposeInternalServerProcess, count)
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
