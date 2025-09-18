package currency

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
)

// CalculateUsdMarketValue calculates the current USD market value of a crypto
// amount in quarks.
func CalculateUsdMarketValue(ctx context.Context, data code_data.Provider, mint *common.Account, quarks uint64, at time.Time) (float64, error) {
	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, at)
	if err != nil {
		return 0, err
	}

	quarksPerUnit := common.GetMintQuarksPerUnit(mint)
	units := float64(quarks) / float64(quarksPerUnit)
	marketValue := usdExchangeRecord.Rate * units
	return marketValue, nil
}
