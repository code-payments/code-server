package async_commitment

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

type service struct {
	log  *logrus.Entry
	data code_data.Provider
}

func New(data code_data.Provider) async.Service {
	return &service{
		log:  logrus.StandardLogger().WithField("service", "commitment"),
		data: data,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {

	// Setup workers to watch for commitment state changes on the Solana side
	for _, item := range []commitment.State{
		commitment.StateReadyToOpen,
		commitment.StateOpen,
		commitment.StateClosed,

		// Note: All other states are handled externally, currently.
	} {
		go func(state commitment.State) {
			err := p.worker(ctx, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("commitment processing loop terminated unexpectedly for state %d", state)
			}

		}(item)
	}

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("treasury metrics gauge loop terminated unexpectedly")
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}
