package async_swap

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/swap"
)

type service struct {
	log  *logrus.Entry
	conf *conf
	data code_data.Provider
}

func New(data code_data.Provider, configProvider ConfigProvider) async.Service {
	return &service{
		log:  logrus.StandardLogger().WithField("service", "swap"),
		conf: configProvider(),
		data: data,
	}

}

func (p *service) Start(ctx context.Context, interval time.Duration) error {

	for _, state := range []swap.State{
		swap.StateFunding,
		swap.StateSubmitting,
	} {
		go func(state swap.State) {

			err := p.worker(ctx, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("swap processing loop terminated unexpectedly for state %s", state.String())
			}

		}(state)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
