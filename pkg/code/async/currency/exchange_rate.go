package async

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

type exchangeRateService struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewExchangeRateService(data code_data.Provider) async.Service {
	return &exchangeRateService{
		log:  logrus.StandardLogger().WithField("service", "exchangerate"),
		data: data,
	}
}

func (p *exchangeRateService) Start(serviceCtx context.Context, interval time.Duration) error {
	// TODO: add distributed lock
	for {
		_, err := retry.Retry(
			func() error {
				p.log.Trace("updating exchange rates")

				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__currency_service")
				defer m.End()
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				err := p.GetCurrentExchangeRates(tracedCtx)
				if err != nil {
					m.NoticeError(err)
					p.log.WithError(err).Warn("failed to process current rate data")
				}

				return err
			},
			retry.NonRetriableErrors(context.Canceled),
			retry.BackoffWithJitter(backoff.BinaryExponential(time.Second), interval, 0.1),
		)
		if err != nil {
			if err != context.Canceled {
				// Should not happen since only non-retriable error is context.Canceled
				p.log.WithError(err).Warn("unexpected error when processing current rate data")
			}

			return err
		}

		select {
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		case <-time.After(interval):
		}
	}
}

func (p *exchangeRateService) GetCurrentExchangeRates(ctx context.Context) error {
	data, err := p.data.GetCurrentExchangeRatesFromExternalProviders(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current rate data")
	}

	if err = p.data.ImportExchangeRates(ctx, data); err != nil {
		return errors.Wrap(err, "failed to store rate data")
	}

	return nil
}
