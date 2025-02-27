package async_airdrop

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/async"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type service struct {
	log             *logrus.Entry
	conf            *conf
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
	integration     Integration

	airdropper                   *common.Account
	airdropperTimelockAccounts   *common.TimelockAccounts
	airdropperMemoryAccount      *common.Account
	airdropperMemoryAccountIndex uint16

	nonceMu                 sync.Mutex
	nextNonceIndex          uint16
	nonceMemoryAccount      *common.Account
	nonceMemoryAccountState *cvm.MemoryAccountWithData
}

func New(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, integration Integration, configProvider ConfigProvider) (async.Service, error) {
	ctx := context.Background()

	s := &service{
		log:             logrus.StandardLogger().WithField("service", "airdrop"),
		conf:            configProvider(),
		data:            data,
		vmIndexerClient: vmIndexerClient,
		integration:     integration,
	}

	err := s.loadAirdropper(ctx)
	if err != nil {
		return nil, err
	}

	err = s.loadNonceMemoryAccount(ctx)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	go func() {
		err := p.airdropWorker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("airdrop processing loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *service) loadAirdropper(ctx context.Context) error {
	log := p.log.WithField("method", "loadAirdropper")

	vaultRecord, err := p.data.GetKey(ctx, p.conf.airdropperOwner.Get(ctx))
	if err != nil {
		log.WithError(err).Warn("failed to load vault record")
		return err
	}

	p.airdropper, err = common.NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
	if err != nil {
		log.WithError(err).Warn("invalid private key")
		return err
	}

	p.airdropperTimelockAccounts, err = p.airdropper.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
	if err != nil {
		log.WithError(err).Warn("failed to dervice timelock accounts")
		return err
	}

	p.airdropperMemoryAccount, p.airdropperMemoryAccountIndex, err = p.getVirtualTimelockAccountMemoryLocation(ctx, common.CodeVmAccount, p.airdropper)
	if err != nil {
		log.WithError(err).Warn("failed to get airdropper memory location")
		return err
	}

	return nil
}

func (p *service) loadNonceMemoryAccount(ctx context.Context) (err error) {
	log := p.log.WithField("method", "loadNonceMemoryAccount")

	p.nextNonceIndex = 0

	p.nonceMemoryAccount, err = common.NewAccountFromPublicKeyString(p.conf.nonceMemoryAccount.Get(ctx))
	if err != nil {
		log.WithError(err).Warn("invalid public key")
		return err
	}

	p.nonceMemoryAccountState = &cvm.MemoryAccountWithData{}

	err = p.refreshNonceMemoryAccountState(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to refresh nonce memory account state")
	}
	return err
}
