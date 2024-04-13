package async_geyser

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/metrics"
)

func (p *service) consumeGeyserProgramUpdateEvents(ctx context.Context) error {
	log := p.log.WithField("method", "consumeGeyserProgramUpdateEvents")

	for {
		// Is the service stopped?
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := p.subscribeToProgramUpdatesFromGeyser(ctx, p.conf.grpcPluginEndpoint.Get(ctx))
		if err != nil && !errors.Is(err, context.Canceled) {
			log.WithError(err).Warn("program update consumer unexpectedly terminated")
		}

		// Avoid spamming new connections when something is wrong
		time.Sleep(time.Second)
	}
}

func (p *service) consumeGeyserSlotUpdateEvents(ctx context.Context) error {
	log := p.log.WithField("method", "consumeGeyserSlotUpdateEvents")

	for {
		// Is the service stopped?
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := p.subscribeToSlotUpdatesFromGeyser(ctx, p.conf.grpcPluginEndpoint.Get(ctx))
		if err != nil && !errors.Is(err, context.Canceled) {
			log.WithError(err).Warn("slot update consumer unexpectedly terminated")
		}

		// Avoid spamming new connections when something is wrong
		time.Sleep(time.Second)
	}
}

func (p *service) programUpdateWorker(serviceCtx context.Context, id int) {
	p.metricStatusLock.Lock()
	_, ok := p.programUpdateWorkerMetrics[id]
	if !ok {
		p.programUpdateWorkerMetrics[id] = &eventWorkerMetrics{}
	}
	p.programUpdateWorkerMetrics[id].active = false
	p.metricStatusLock.Unlock()

	log := p.log.WithFields(logrus.Fields{
		"method":    "programUpdateWorker",
		"worker_id": id,
	})

	log.Debug("worker started")

	defer func() {
		log.Debug("worker stopped")
	}()

	for update := range p.programUpdatesChan {
		func() {
			nr := serviceCtx.Value(metrics.NewRelicContextKey{}).(*newrelic.Application)
			m := nr.StartTransaction("async__geyser_consumer_service__program_update_worker")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			p.metricStatusLock.Lock()
			p.programUpdateWorkerMetrics[id].active = true
			p.metricStatusLock.Unlock()
			defer func() {
				p.metricStatusLock.Lock()
				p.programUpdateWorkerMetrics[id].active = false
				p.metricStatusLock.Unlock()
			}()

			publicKey, err := common.NewAccountFromPublicKeyBytes(update.Pubkey)
			if err != nil {
				log.WithError(err).Warn("invalid public key")
				return
			}

			program, err := common.NewAccountFromPublicKeyBytes(update.Owner)
			if err != nil {
				log.WithError(err).Warn("invalid owner account")
				return
			}

			log := log.WithFields(logrus.Fields{
				"program": program.PublicKey().ToBase58(),
				"account": publicKey.PublicKey().ToBase58(),
				"slot":    update.Slot,
			})
			if update.TxSignature != nil {
				log = log.WithField("transaction", *update.TxSignature)
			}

			handler, ok := p.programUpdateHandlers[program.PublicKey().ToBase58()]
			if !ok {
				log.Debug("not handling update from program")
				return
			}

			err = handler.Handle(tracedCtx, update)
			if err != nil {
				m.NoticeError(err)
				log.WithError(err).Warn("failed to process program account update")
			}

			p.metricStatusLock.Lock()
			p.programUpdateWorkerMetrics[id].eventsProcessed++
			p.metricStatusLock.Unlock()
		}()
	}
}
