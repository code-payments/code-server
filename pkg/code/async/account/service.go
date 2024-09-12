package async_account

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

type service struct {
	log    *logrus.Entry
	conf   *conf
	data   code_data.Provider
	pusher push_lib.Provider
}

func New(data code_data.Provider, pusher push_lib.Provider, configProvider ConfigProvider) async.Service {
	return &service{
		log:    logrus.StandardLogger().WithField("service", "account"),
		conf:   configProvider(),
		data:   data,
		pusher: pusher,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	// todo: auto returns are broken because we've removed close dormant account actions
	/*
		go func() {
			err := p.giftCardAutoReturnWorker(ctx, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warn("gift card auto-return processing loop terminated unexpectedly")
			}
		}()
	*/

	go func() {
		err := p.swapRetryWorker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("swap retry processing loop terminated unexpectedly")
		}
	}()

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("account metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
