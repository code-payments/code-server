package async_nonce

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/async"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
)

var (
	ErrInvalidNonceAccountSize   = errors.New("invalid nonce account size")
	ErrInvalidNonceLimitExceeded = errors.New("nonce account limit exceeded")
	ErrNoAvailableKeys           = errors.New("no available keys in the vault")
)

type service struct {
	log             *logrus.Entry
	conf            *conf
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	rent uint64
}

func New(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, configProvider ConfigProvider) async.Service {
	return &service{
		log:             logrus.StandardLogger().WithField("service", "nonce"),
		conf:            configProvider(),
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	// Generate vault keys until we have a minimum in reserve to use for the pool
	// on Solana mainnet
	go p.generateKeys(ctx)

	// Watch the size of the Solana mainnet nonce pool and create accounts if necessary
	go p.generateNonceAccountsOnSolanaMainnet(ctx)

	// Setup workers to watch for nonce state changes on the Solana side
	for _, item := range []nonce.State{
		nonce.StateUnknown,
		nonce.StateReleased,
	} {
		go func(state nonce.State) {

			err := p.worker(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("nonce processing loop terminated unexpectedly for state %d", state)
			}

		}(item)
	}

	// Setup workers to watch for nonce state changes on the CVM side
	for _, item := range []nonce.State{
		nonce.StateReleased,
	} {
		go func(state nonce.State) {

			err := p.worker(ctx, nonce.EnvironmentCvm, common.CodeVmAccount.PublicKey().ToBase58(), state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("nonce processing loop terminated unexpectedly for state %d", state)
			}

		}(item)
	}

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("nonce metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
