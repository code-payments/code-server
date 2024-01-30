package async_geyser

import (
	"context"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/metrics"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
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
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				jobSucceeded := true

				// Find and process unlocked timelock accounts unlocking between [+21 - n days, +21 days],
				// which enables a retry mechanism across time.
				for i := uint8(0); i <= uint8(p.conf.backupTimelockWorkerDaysChecked.Get(tracedCtx)); i++ {
					daysUntilUnlock := timelock_token_v1.DefaultNumDaysLocked - i

					addresses, slot, err := findUnlockedTimelockV1Accounts(tracedCtx, p.data, daysUntilUnlock)
					if err != nil {
						m.NoticeError(err)
						log.WithError(err).Warn("failure getting unlocked timelock accounts")
						jobSucceeded = false
						continue
					}

					log.Infof("found %d timelock accounts unlocking in %d days", len(addresses), daysUntilUnlock)

					for _, address := range addresses {
						log := log.WithField("account", address)

						stateAccount, err := common.NewAccountFromPublicKeyString(address)
						if err != nil {
							log.WithError(err).Warn("invalid state account address")
							continue
						}

						err = updateTimelockV1AccountCachedState(tracedCtx, p.data, stateAccount, slot)
						if err != nil {
							m.NoticeError(err)
							log.WithError(err).Warn("failure updating cached timelock account state")
							jobSucceeded = false
							continue
						}
					}
				}

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
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				accountInfoRecords, err := p.data.GetPrioritizedAccountInfosRequiringDepositSync(tracedCtx, p.conf.backupExternalDepositWorkerCount.Get(tracedCtx))
				if err != nil {
					if err != account.ErrAccountInfoNotFound {
						m.NoticeError(err)
						log.WithError(err).Warn("failure getting accounts to sync external deposits")
					}
					return
				}

				var wg sync.WaitGroup
				for _, accountInfoRecord := range accountInfoRecords {
					vault, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
					if err != nil {
						log.WithError(err).WithField("account", accountInfoRecord.TokenAccount).Warn("invalid token account")
						continue
					}

					wg.Add(1)

					go func(vault *common.Account) {
						defer wg.Done()

						log := log.WithField("account", vault.PublicKey().ToBase58())

						err := fixMissingExternalDeposits(tracedCtx, p.conf, p.data, p.pusher, vault)
						if err != nil {
							m.NoticeError(err)
							log.WithError(err).Warn("failure fixing missing external deposits")
						}
					}(vault)
				}
				wg.Wait()
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
