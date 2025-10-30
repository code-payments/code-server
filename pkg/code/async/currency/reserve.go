package async

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
	"github.com/code-payments/code-server/pkg/solana/token"
)

type reserveService struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewReserveService(data code_data.Provider) async.Service {
	return &reserveService{
		log:  logrus.StandardLogger().WithField("service", "reserve"),
		data: data,
	}
}

func (p *reserveService) Start(serviceCtx context.Context, interval time.Duration) error {
	for {
		_, err := retry.Retry(
			func() error {
				p.log.Trace("updating exchange rates")

				nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
				m := nr.StartTransaction("async__currency_reserve_service")
				defer m.End()
				tracedCtx := newrelic.NewContext(serviceCtx, m)

				err := p.UpdateAllLaunchpadCurrencyReserves(tracedCtx)
				if err != nil {
					m.NoticeError(err)
					p.log.WithError(err).Warn("failed to process current rate data")
				}

				return err
			},
			retry.NonRetriableErrors(context.Canceled),
			retry.BackoffWithJitter(backoff.BinaryExponential(time.Second), interval, 0.1),
		)
		if err != nil {
			if err != context.Canceled {
				// Should not happen since only non-retriable error is context.Canceled
				p.log.WithError(err).Warn("unexpected error when processing current reserve data")
			}

			return err
		}

		select {
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		case <-time.After(interval):
		}
	}
}

// todo: Don't hardcode Jeffy
func (p *reserveService) UpdateAllLaunchpadCurrencyReserves(ctx context.Context) error {
	err1 := func() error {
		jeffyMintAccount, _ := common.NewAccountFromPublicKeyString("52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj")
		jeffyVaultAccount, _ := common.NewAccountFromPublicKeyString("BFDanLgELhpCCGTtaa7c8WGxTXcTxgwkf9DMQd4qheSK")
		coreMintVaultAccount, _ := common.NewAccountFromPublicKeyString("A9NVHVuorNL4y2YFxdwdU3Hqozxw1Y1YJ81ZPxJsRrT4")

		var tokenAccount token.Account
		ai, err := p.data.GetBlockchainAccountInfo(ctx, jeffyVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
		if err != nil {
			return err
		}
		tokenAccount.Unmarshal(ai.Data)
		jeffyVaultBalance := tokenAccount.Amount

		ai, err = p.data.GetBlockchainAccountInfo(ctx, coreMintVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
		if err != nil {
			return err
		}
		tokenAccount.Unmarshal(ai.Data)
		coreMintVaultBalance := tokenAccount.Amount

		return p.data.PutCurrencyReserve(ctx, &currency.ReserveRecord{
			Mint:              jeffyMintAccount.PublicKey().ToBase58(),
			SupplyFromBonding: currencycreator.DefaultMintMaxQuarkSupply - jeffyVaultBalance,
			CoreMintLocked:    coreMintVaultBalance,
			Time:              time.Now(),
		})
	}()

	err2 := func() error {
		knicksNightMintAccount, _ := common.NewAccountFromPublicKeyString("497Wy6cY9BjWBiaDHzJ7TcUZqF2gE1Qm7yXtSj1vSr5W")
		knicksNightVaultAccount, _ := common.NewAccountFromPublicKeyString("GEJGcTHfggJ4P82AwrmNWji2AkLq5eRUDM2hQSZ5SXpt")
		coreMintVaultAccount, _ := common.NewAccountFromPublicKeyString("AZN7RinWLBtjxtJL6rLxFgb2rtcbpUJ67pfcq71Z3mKk")

		var tokenAccount token.Account
		ai, err := p.data.GetBlockchainAccountInfo(ctx, knicksNightVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
		if err != nil {
			return err
		}
		tokenAccount.Unmarshal(ai.Data)
		knicksNightVaultBalance := tokenAccount.Amount

		ai, err = p.data.GetBlockchainAccountInfo(ctx, coreMintVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
		if err != nil {
			return err
		}
		tokenAccount.Unmarshal(ai.Data)
		coreMintVaultBalance := tokenAccount.Amount

		return p.data.PutCurrencyReserve(ctx, &currency.ReserveRecord{
			Mint:              knicksNightMintAccount.PublicKey().ToBase58(),
			SupplyFromBonding: currencycreator.DefaultMintMaxQuarkSupply - knicksNightVaultBalance,
			CoreMintLocked:    coreMintVaultBalance,
			Time:              time.Now(),
		})
	}()

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}

	return nil
}
