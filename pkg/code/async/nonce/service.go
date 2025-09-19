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
	for _, state := range []nonce.State{
		nonce.StateUnknown,
		nonce.StateReleased,
	} {
		go func(state nonce.State) {

			err := p.worker(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state, interval)
			if err != nil && err != context.Canceled {
				p.log.WithError(err).Warnf("nonce processing loop terminated unexpectedly for env %s, instance %s, state %d", nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, state)
			}

		}(state)
	}

	// Setup workers to watch for nonce state changes on the CVM side
	//
	// todo: Dynamically detect VMs
	for _, vm := range []string{
		common.CodeVmAccount.PublicKey().ToBase58(),
		"Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta",
	} {
		for _, state := range []nonce.State{
			nonce.StateReleased,
		} {
			go func(vm string, state nonce.State) {

				err := p.worker(ctx, nonce.EnvironmentCvm, vm, state, interval)
				if err != nil && err != context.Canceled {
					p.log.WithError(err).Warnf("nonce processing loop terminated unexpectedly for env %s, instance %s, state %d", nonce.EnvironmentCvm, vm, state)
				}

			}(vm, state)
		}
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
