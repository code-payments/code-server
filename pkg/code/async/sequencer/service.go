package async_sequencer

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/transaction"
)

var (
	ErrInvalidFulfillmentSignature       = errors.New("invalid fulfillment signature")
	ErrInvalidFulfillmentStateTransition = errors.New("invalid fulfillment state transition")
	ErrCouldNotGetIntentLock             = errors.New("could not get intent lock")
)

type service struct {
	log                       *logrus.Entry
	conf                      *conf
	data                      code_data.Provider
	scheduler                 Scheduler
	vmIndexerClient           indexerpb.IndexerClient
	noncePool                 *transaction.LocalNoncePool
	fulfillmentHandlersByType map[fulfillment.Type]FulfillmentHandler
	actionHandlersByType      map[action.Type]ActionHandler
	intentHandlersByType      map[intent.Type]IntentHandler
}

func New(data code_data.Provider, scheduler Scheduler, vmIndexerClient indexerpb.IndexerClient, noncePool *transaction.LocalNoncePool, configProvider ConfigProvider) (async.Service, error) {
	noncePoolEnv, noncePoolEnvInstance, noncePoolType := noncePool.GetConfiguration()
	if noncePoolEnv != nonce.EnvironmentSolana {
		return nil, errors.Errorf("nonce pool environment must be %s", nonce.EnvironmentSolana)
	}
	if noncePoolEnvInstance != nonce.EnvironmentInstanceSolanaMainnet {
		return nil, errors.Errorf("nonce pool environment instance must be %s", nonce.EnvironmentInstanceSolanaMainnet)
	}
	if noncePoolType != nonce.PurposeOnDemandTransaction {
		return nil, errors.Errorf("nonce pool type must be %s", nonce.PurposeOnDemandTransaction)
	}

	return &service{
		log:                       logrus.StandardLogger().WithField("service", "sequencer"),
		conf:                      configProvider(),
		data:                      data,
		scheduler:                 scheduler,
		vmIndexerClient:           vmIndexerClient,
		noncePool:                 noncePool, // todo: validate configuration
		fulfillmentHandlersByType: getFulfillmentHandlers(data, vmIndexerClient),
		actionHandlersByType:      getActionHandlers(data),
		intentHandlersByType:      getIntentHandlers(data),
	}, nil
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {

	// Setup workers to watch for fulfillment state changes on the Solana side
	for _, item := range []fulfillment.State{
		fulfillment.StateUnknown,
		fulfillment.StatePending,

		// There's no executable logic for these states yet:
		// fulfillment.StateConfirmed,
		// fulfillment.StateFailed,
		// fulfillment.StateRevoked,
	} {
		go func(state fulfillment.State) {

			// todo: Note to our future selves that there are some components of
			//       the scheduler (ie. subsidizer balance checks) that won't
			//       work perfectly in a multi-threaded or multi-node environment.
			err := p.worker(ctx, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("fulfillment processing loop terminated unexpectedly for state %d", state)
			}

		}(item)
	}

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("fulfillment metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
