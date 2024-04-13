package async_geyser

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	geyserpb "github.com/code-payments/code-server/pkg/code/async/geyser/api/gen"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

type eventWorkerMetrics struct {
	active          bool
	eventsProcessed int
}

type service struct {
	log    *logrus.Entry
	data   code_data.Provider
	pusher push_lib.Provider
	conf   *conf

	programUpdatesChan    chan *geyserpb.AccountUpdate
	programUpdateHandlers map[string]ProgramAccountUpdateHandler

	metricStatusLock sync.RWMutex

	programUpdateSubscriptionStatus bool
	programUpdateWorkerMetrics      map[int]*eventWorkerMetrics

	slotUpdateSubscriptionStatus bool
	highestObservedRootedSlot    uint64

	backupTimelockStateWorkerStatus bool
	unlockedTimelockAccountsSynced  bool

	backupExternalDepositWorkerStatus bool

	backupMessagingWorkerStatus bool
}

func New(data code_data.Provider, pusher push_lib.Provider, configProvider ConfigProvider) async.Service {
	conf := configProvider()
	return &service{
		log:                        logrus.StandardLogger().WithField("service", "geyser_consumer"),
		data:                       data,
		pusher:                     pusher,
		conf:                       configProvider(),
		programUpdatesChan:         make(chan *geyserpb.AccountUpdate, conf.programUpdateQueueSize.Get(context.Background())),
		programUpdateHandlers:      initializeProgramAccountUpdateHandlers(conf, data, pusher),
		programUpdateWorkerMetrics: make(map[int]*eventWorkerMetrics),
	}
}

func (p *service) Start(ctx context.Context, _ time.Duration) error {
	// Start backup workers to catch missed events
	go func() {
		err := p.backupTimelockStateWorker(ctx, p.conf.backupTimelockWorkerInterval.Get(ctx))
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("timelock backup worker terminated unexpectedly")
		}
	}()
	go func() {
		err := p.backupExternalDepositWorker(ctx, p.conf.backupExternalDepositWorkerInterval.Get(ctx))
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("external deposit backup worker terminated unexpectedly")
		}
	}()
	go func() {
		err := p.backupMessagingWorker(ctx, p.conf.backupMessagingWorkerInterval.Get(ctx))
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("messaging backup worker terminated unexpectedly")
		}
	}()

	// Setup event worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < int(p.conf.programUpdateWorkerCount.Get(ctx)); i++ {
		wg.Add(1)
		go func(id int) {
			p.programUpdateWorker(ctx, id)
			wg.Done()
		}(i)
	}

	// Main event loops to consume updates from subscriptions to Geyser that
	// will be processed async
	go func() {
		err := p.consumeGeyserProgramUpdateEvents(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("geyser event consumer terminated unexpectedly")
		}
	}()
	go func() {
		err := p.consumeGeyserSlotUpdateEvents(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("geyser event consumer terminated unexpectedly")
		}
	}()

	// Start metrics gauge worker
	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("metrics gauge loop terminated unexpectedly")
		}
	}()

	// Wait for the service to stop
	<-ctx.Done()

	// Gracefully shutdown
	close(p.programUpdatesChan)
	wg.Wait()

	return nil
}
