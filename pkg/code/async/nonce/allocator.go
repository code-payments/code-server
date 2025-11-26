package async_nonce

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
)

// todo: Add process for allocating VDN, which has some key differences:
// - Don't know the address in advance
// - Need some level of memory account management with the ability to find a free index
// - Does not require a vault key record

func (p *service) generateNonceAccountsOnSolanaMainnet(serviceCtx context.Context, purpose nonce.Purpose, desiredPoolSize uint64) error {

	hasWarnedUser := false
	err := retry.Loop(
		func() (err error) {
			time.Sleep(time.Second)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__nonce_service__nonce_accounts")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			num_invalid, err := p.data.GetNonceCountByStateAndPurpose(tracedCtx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateInvalid, purpose)
			if err != nil {
				return err
			}

			// prevent infinite nonce creation
			if num_invalid > 100 {
				return ErrInvalidNonceLimitExceeded
			}

			num_available, err := p.data.GetNonceCountByStateAndPurpose(tracedCtx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateAvailable, purpose)
			if err != nil {
				return err
			}

			num_claimed, err := p.data.GetNonceCountByStateAndPurpose(tracedCtx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateClaimed, purpose)
			if err != nil {
				return err
			}

			num_released, err := p.data.GetNonceCountByStateAndPurpose(tracedCtx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateReleased, purpose)
			if err != nil {
				return err
			}

			num_unknown, err := p.data.GetNonceCountByStateAndPurpose(tracedCtx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown, purpose)
			if err != nil {
				return err
			}

			// Get a count of nonces that are available or potentially available
			// within a short amount of time.
			num_potentially_available := num_available + num_claimed + num_released + num_unknown
			if num_potentially_available >= desiredPoolSize {
				if hasWarnedUser {
					p.log.Warn("The nonce pool size is reached.")
					hasWarnedUser = false
				}
				return nil
			}

			if !hasWarnedUser {
				hasWarnedUser = true
				p.log.Warn("The nonce pool is too small.")
			}

			_, err = p.createSolanaMainnetNonce(tracedCtx)
			if err != nil {
				p.log.WithError(err).Warn("failure creating nonce")
				return err
			}

			return nil

		},
		retry.NonRetriableErrors(context.Canceled, ErrInvalidNonceLimitExceeded),
	)

	return err
}
