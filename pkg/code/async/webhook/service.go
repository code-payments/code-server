package async_webhook

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

type service struct {
	log             *logrus.Entry
	conf            *conf
	data            code_data.Provider
	messagingClient messaging.InternalMessageClient
	webhookLocks    *sync_util.StripedLock // todo: distributed lock

	metricsMu          sync.Mutex
	successfulWebhooks int
	failedWebhooks     int
}

func New(data code_data.Provider, messagingClient messaging.InternalMessageClient, configProvider ConfigProvider) async.Service {
	return &service{
		log:             logrus.StandardLogger().WithField("service", "webhook"),
		conf:            configProvider(),
		data:            data,
		messagingClient: messagingClient,
		webhookLocks:    sync_util.NewStripedLock(1024),
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	go func() {
		err := p.worker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warnf("webhook processing loop terminated unexpectedly")
		}
	}()

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("webhook metrics gauge loop terminated unexpectedly")
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}
