package async_airdrop

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/metrics"
)

func (p *service) airdropWorker(serviceCtx context.Context, interval time.Duration) error {
	delay := time.After(interval)

	for {
		select {
		case <-delay:
			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__airdrop_service__airdrop")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			start := time.Now()

			err := p.doAirdrop(tracedCtx)
			if err != nil {
				m.NoticeError(err)
			}

			delay = time.After(interval - time.Since(start))
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		}
	}
}

func (p *service) doAirdrop(ctx context.Context) error {
	log := p.log.WithField("method", "doAirdrop")

	err := p.refreshNonceMemoryAccountState(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to refresh nonce memory account state")
		return err
	}

	owners, amount, err := p.integration.GetOwnersToAirdropNow(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get owners to airdrop to")
		return err
	}

	if len(owners) == 0 {
		return nil
	}

	err = p.airdropToOwners(ctx, amount, owners...)
	if err != nil {
		log.WithError(err).Warn("failed to airdrop to owners")
		return err
	}

	return nil
}
