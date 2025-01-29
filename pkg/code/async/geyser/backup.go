package async_geyser

import (
	"context"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
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
	cursor := query.EmptyCursor
	oldestRecordTs := time.Now()
	for {
		select {
		case <-time.After(delay):
			start := time.Now()

			func() {
				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__geyser_consumer_service__backup_timelock_state_worker")
				defer m.End()
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				timelockRecords, err := p.data.GetAllTimelocksByState(
					tracedCtx,
					timelock_token.StateLocked,
					query.WithDirection(query.Ascending),
					query.WithCursor(cursor),
					query.WithLimit(256),
				)
				if err == timelock.ErrTimelockNotFound {
					p.metricStatusLock.Lock()
					copiedTs := oldestRecordTs
					p.oldestTimelockRecord = &copiedTs
					p.metricStatusLock.Unlock()

					cursor = query.EmptyCursor
					oldestRecordTs = time.Now()
					return
				} else if err != nil {
					log.WithError(err).Warn("failed to get timelock records")
					return
				}

				var wg sync.WaitGroup
				for _, timelockRecord := range timelockRecords {
					wg.Add(1)

					if timelockRecord.LastUpdatedAt.Before(oldestRecordTs) {
						oldestRecordTs = timelockRecord.LastUpdatedAt
					}

					go func(timelockRecord *timelock.Record) {
						defer wg.Done()

						log := log.WithField("timelock", timelockRecord.Address)

						err := updateTimelockAccountRecord(tracedCtx, p.data, timelockRecord)
						if err != nil {
							log.WithError(err).Warn("failed to update timelock account")
						}
					}(timelockRecord)
				}

				wg.Wait()

				cursor = query.ToCursor(timelockRecords[len(timelockRecords)-1].Id)
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
