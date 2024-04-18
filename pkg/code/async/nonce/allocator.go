package async_nonce

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
)

func (p *service) generateNonceAccounts(serviceCtx context.Context) error {
	hasWarnedUser := false
	err := retry.Loop(
		func() (err error) {
			time.Sleep(time.Second)

			nr := serviceCtx.Value(metrics.NewRelicContextKey{}).(*newrelic.Application)
			m := nr.StartTransaction("async__nonce_service__nonce_accounts")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			num_invalid, err := p.data.GetNonceCountByState(tracedCtx, nonce.StateInvalid)
			if err != nil {
				return err
			}

			// prevent infinite nonce creation
			if num_invalid > 100 {
				return ErrInvalidNonceLimitExceeded
			}

			num_available, err := p.data.GetNonceCountByState(tracedCtx, nonce.StateAvailable)
			if err != nil {
				return err
			}

			num_released, err := p.data.GetNonceCountByState(tracedCtx, nonce.StateReleased)
			if err != nil {
				return err
			}

			num_unknown, err := p.data.GetNonceCountByState(tracedCtx, nonce.StateUnknown)
			if err != nil {
				return err
			}

			// Get a count of nonces that are available or potentially available
			// within a short amount of time.
			num_potentially_available := num_available + num_released + num_unknown
			if num_potentially_available >= uint64(p.size) {
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

			_, err = p.createNonce(tracedCtx)
			if err != nil {
				return err
			}

			return nil
		},
		retry.NonRetriableErrors(context.Canceled, ErrInvalidNonceLimitExceeded),
	)

	return err
}
