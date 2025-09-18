package currency

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
)

// CalculateUsdMarketValue calculates the current USD market value of a crypto
// amount in quarks.
func CalculateUsdMarketValue(ctx context.Context, data code_data.Provider, mint *common.Account, quarks uint64, at time.Time) (float64, float64, error) {
	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, at)
	if err != nil {
		return 0, 0, err
	}

	pricePerCoreMint := 1.0
	if mint.PublicKey().ToBase58() != common.CoreMintAccount.PublicKey().ToBase58() {
		reserveRecord, err := data.GetCurrencyReserveAtTime(ctx, mint.PublicKey().ToBase58(), at)
		if err != nil {
			return 0, 0, err
		}

		pricePerCoreMint, _ = currencycreator.EstimateCurrentPrice(reserveRecord.SupplyFromBonding).Float64()
	}

	rate := pricePerCoreMint * usdExchangeRecord.Rate

	quarksPerUnit := common.GetMintQuarksPerUnit(mint)
	units := float64(quarks) / float64(quarksPerUnit)
	marketValue := usdExchangeRecord.Rate * units
	return marketValue, rate, nil
}
