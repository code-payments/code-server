package async_swap

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/swap"
)

type service struct {
	log             *logrus.Entry
	conf            *conf
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
	integration     Integration
}

func New(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, integration Integration, configProvider ConfigProvider) async.Service {
	return &service{
		log:             logrus.StandardLogger().WithField("service", "swap"),
		conf:            configProvider(),
		data:            data,
		vmIndexerClient: vmIndexerClient,
		integration:     integration,
	}

}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	for _, state := range []swap.State{
		swap.StateCreated,
		swap.StateFunding,
		swap.StateFunded,
		swap.StateSubmitting,
		swap.StateCancelling,
	} {
		go func(state swap.State) {

			err := p.worker(ctx, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("swap processing loop terminated unexpectedly for state %s", state.String())
			}

		}(state)
	}

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("swap metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
