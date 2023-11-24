package async_treasury

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
)

type service struct {
	log  *logrus.Entry
	conf *conf
	data code_data.Provider
}

func New(data code_data.Provider, configProvider ConfigProvider) async.Service {
	return &service{
		log:  logrus.StandardLogger().WithField("service", "treasury"),
		conf: configProvider(),
		data: data,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	// Setup workers to watch for updates to pools
	for _, item := range []treasury.TreasuryPoolState{
		treasury.TreasuryPoolStateAvailable,

		// Below states have no executable logic
		// treasury.TreasuryPoolStateUnknown,
		// treasury.TreasuryPoolStateDeprecated,
	} {
		go func(state treasury.TreasuryPoolState) {

			err := p.worker(ctx, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("pool processing loop terminated unexpectedly for state %d", state)
			}

		}(item)
	}

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("treasury metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
