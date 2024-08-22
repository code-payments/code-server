package async_geyser

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/metrics"
)

// Backup system workers can be found here. This is necessary because we can't rely
// on receiving all updates from Geyser. As a result, we should design backup systems
// to assume Geyser doesn't function/exist at all. Why do we need Geyser if this is
// the case? Real time updates. Backup workers likely won't be able to guarantee
// real time (or near real time) updates at scale.

func (p *service) backupTimelockStateWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "backupTimelockStateWorker")
	log.Debug("worker started")

	p.metricStatusLock.Lock()
	p.backupTimelockStateWorkerStatus = true
	p.metricStatusLock.Unlock()
	defer func() {
		p.metricStatusLock.Lock()
		p.backupTimelockStateWorkerStatus = false
		p.metricStatusLock.Unlock()

		log.Debug("worker stopped")
	}()

	delay := 0 * time.Second // Initially no delay, so we can run right after a deploy
	for {
		select {
		case <-time.After(delay):
			start := time.Now()

			func() {
				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__geyser_consumer_service__backup_timelock_state_worker")
				defer m.End()
				//tracedCtx := newrelic.NewContext(serviceCtx, m)

				jobSucceeded := false

				// todo: implement me

				p.metricStatusLock.Lock()
				p.unlockedTimelockAccountsSynced = jobSucceeded
				p.metricStatusLock.Unlock()
			}()

			delay = interval - time.Since(start)
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		}
	}
}

func (p *service) backupExternalDepositWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "backupExternalDepositWorker")
	log.Debug("worker started")

	p.metricStatusLock.Lock()
	p.backupExternalDepositWorkerStatus = true
	p.metricStatusLock.Unlock()
	defer func() {
		p.metricStatusLock.Lock()
		p.backupExternalDepositWorkerStatus = false
		p.metricStatusLock.Unlock()

		log.Debug("worker stopped")
	}()

	for {
		select {
		case <-time.After(interval):
			func() {
				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__geyser_consumer_service__backup_external_deposit_worker")
				defer m.End()
				// tracedCtx := newrelic.NewContext(serviceCtx, m)

				// todo: implement me
			}()
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		}
	}
}

func (p *service) backupMessagingWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "backupMessagingWorker")
	log.Debug("worker started")

	p.metricStatusLock.Lock()
	p.backupMessagingWorkerStatus = true
	p.metricStatusLock.Unlock()
	defer func() {
		p.metricStatusLock.Lock()
		p.backupMessagingWorkerStatus = false
		p.metricStatusLock.Unlock()

		log.Debug("worker stopped")
	}()

	delay := 0 * time.Second // Initially no delay, so we can run right after a deploy

	messagingFeeCollector, err := common.NewAccountFromPublicKeyString(p.conf.messagingFeeCollectorPublicKey.Get(serviceCtx))
	if err != nil {
		return err
	}

	var checkpoint *string
	for {
		select {
		case <-time.After(delay):
			start := time.Now()

			func() {
				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__geyser_consumer_service__backup_messaging_worker")
				defer m.End()
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				checkpoint, err = fixMissingBlockchainMessages(tracedCtx, p.data, p.pusher, messagingFeeCollector, checkpoint)
				if err != nil {
					m.NoticeError(err)
					log.WithError(err).Warn("failure fixing missing messages")
				}
			}()

			delay = interval - time.Since(start)
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		}
	}
}
