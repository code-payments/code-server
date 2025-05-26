package async_account

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

type service struct {
	log  *logrus.Entry
	conf *conf
	data code_data.Provider

	airdropper *common.TimelockAccounts
}

func New(data code_data.Provider, configProvider ConfigProvider) async.Service {
	ctx := context.Background()

	p := &service{
		log:  logrus.StandardLogger().WithField("service", "account"),
		conf: configProvider(),
		data: data,
	}

	airdropper := p.conf.airdropperOwnerPublicKey.Get(ctx)
	if len(airdropper) > 0 && airdropper != defaultAirdropperOwnerPublicKey {
		p.mustLoadAirdropper(ctx)
	}

	return p
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {

	go func() {
		err := p.giftCardAutoReturnWorker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("gift card auto-return processing loop terminated unexpectedly")
		}
	}()

	go func() {
		err := p.metricsGaugeWorker(ctx)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("account metrics gauge loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *service) mustLoadAirdropper(ctx context.Context) {
	log := p.log.WithFields(logrus.Fields{
		"method": "mustLoadAirdropper",
		"key":    p.conf.airdropperOwnerPublicKey.Get(ctx),
	})

	err := func() error {
		vaultRecord, err := p.data.GetKey(ctx, p.conf.airdropperOwnerPublicKey.Get(ctx))
		if err != nil {
			return err
		}

		ownerAccount, err := common.NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
		if err != nil {
			return err
		}

		timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
		if err != nil {
			return err
		}

		p.airdropper = timelockAccounts
		return nil
	}()
	if err != nil {
		log.WithError(err).Fatal("failure loading account")
	}
}
