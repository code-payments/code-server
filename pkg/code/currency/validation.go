package currency

import (
	"context"
	"math/big"
	"time"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
)

const (
	defaultPrecision = 128
)

// ValidateClientExchangeData validates proto exchange data provided by a client
func ValidateClientExchangeData(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) (bool, string, error) {
	mint, err := common.GetBackwardsCompatMint(proto.Mint)
	if err != nil {
		return false, "", err
	}

	if common.IsCoreMint(mint) {
		return validateCoreMintClientExchangeData(ctx, data, proto)
	}
	return validateCurrencyLaunchpadClientExchangeData(ctx, data, proto)
}

func validateCoreMintClientExchangeData(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) (bool, string, error) {
	latestExchangeRateTime := GetLatestExchangeRateTime()

	clientRate := big.NewFloat(proto.ExchangeRate).SetPrec(defaultPrecision)
	clientNativeAmount := big.NewFloat(proto.NativeAmount).SetPrec(defaultPrecision)
	clientQuarks := big.NewFloat(float64(proto.Quarks)).SetPrec(defaultPrecision)

	rateErrorThreshold := big.NewFloat(0.01).SetPrec(defaultPrecision)
	quarkErrorThreshold := big.NewFloat(1000).SetPrec(defaultPrecision)

	// Find an exchange rate that the client could have fetched from a RPC call
	// within a reasonable time in the past
	var isClientRateValid bool
	for i := range 2 {
		exchangeRateTime := latestExchangeRateTime.Add(time.Duration(-i) * timePerExchangeRateUpdate)

		exchangeRateRecord, err := data.GetExchangeRate(ctx, currency_lib.Code(proto.Currency), exchangeRateTime)
		if err == currency.ErrNotFound {
			continue
		} else if err != nil {
			return false, "", err
		}
		actualRate := big.NewFloat(exchangeRateRecord.Rate)

		percentDiff := new(big.Float).Quo(new(big.Float).Abs(new(big.Float).Sub(clientRate, actualRate)), actualRate)
		if percentDiff.Cmp(rateErrorThreshold) < 0 {
			isClientRateValid = true
			break
		}
	}

	if !isClientRateValid {
		return false, "fiat exchange rate is stale or invalid", nil
	}

	// Validate that the native amount and exchange rate fall reasonably within
	// the amount of quarks to send in the transaction.
	quarksPerUnit := big.NewFloat(float64(common.GetMintQuarksPerUnit(common.CoreMintAccount)))
	unitsOfCoreMint := new(big.Float).Quo(clientNativeAmount, clientRate)
	expectedQuarks := new(big.Float).Mul(unitsOfCoreMint, quarksPerUnit)
	diff := new(big.Float).Abs(new(big.Float).Sub(expectedQuarks, clientQuarks))
	if diff.Cmp(quarkErrorThreshold) > 1000 {
		return false, "payment native amount and quark value mismatch", nil
	}

	return true, "", nil
}

func validateCurrencyLaunchpadClientExchangeData(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) (bool, string, error) {
	mintAccount, err := common.GetBackwardsCompatMint(proto.Mint)
	if err != nil {
		return false, "", err
	}

	coreMintQuarksPerUnit := common.GetMintQuarksPerUnit(common.CoreMintAccount)
	otherMintQuarksPerUnit := common.GetMintQuarksPerUnit(mintAccount)

	clientQuarks := big.NewFloat(float64(proto.Quarks)).SetPrec(defaultPrecision)
	clientTokenUnits := new(big.Float).Quo(
		clientQuarks,
		big.NewFloat(float64(otherMintQuarksPerUnit)).SetPrec(defaultPrecision),
	)
	clientRate := big.NewFloat(proto.ExchangeRate).SetPrec(defaultPrecision)
	clientNativeAmount := big.NewFloat(proto.NativeAmount).SetPrec(defaultPrecision)

	rateErrorThreshold := big.NewFloat(0.01).SetPrec(defaultPrecision)
	nativeAmountErrorThreshold := big.NewFloat(0.005).SetPrec(defaultPrecision)

	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"currency":             proto.Currency,
		"client_native_amount": clientNativeAmount,
		"client_exchange_rate": clientRate,
		"client_token_units":   clientTokenUnits,
		"client_quarks":        proto.Quarks,
		"mint":                 mintAccount.PublicKey().ToBase58(),
	})

	latestExchangeRateTime := GetLatestExchangeRateTime()
	for i := range 2 {
		exchangeRateTime := latestExchangeRateTime.Add(time.Duration(-i) * timePerExchangeRateUpdate)

		reserveRecord, err := data.GetCurrencyReserveAtTime(ctx, mintAccount.PublicKey().ToBase58(), exchangeRateTime)
		if err == currency.ErrNotFound {
			continue
		} else if err != nil {
			return false, "", err
		}

		usdExchangeRateRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, exchangeRateTime)
		if err == currency.ErrNotFound {
			continue
		} else if err != nil {
			return false, "", err
		}
		usdRate := big.NewFloat(usdExchangeRateRecord.Rate).SetPrec(defaultPrecision)

		var otherExchangeRateRecord *currency.ExchangeRateRecord
		if proto.Currency == string(currency_lib.USD) {
			otherExchangeRateRecord = usdExchangeRateRecord
		} else {
			otherExchangeRateRecord, err = data.GetExchangeRate(ctx, currency_lib.Code(proto.Currency), exchangeRateTime)
			if err == currency.ErrNotFound {
				continue
			} else if err != nil {
				return false, "", err
			}
		}
		otherRate := big.NewFloat(otherExchangeRateRecord.Rate).SetPrec(defaultPrecision)

		// How much core mint would be received for a sell against the currency creator program?
		coreMintSellValueInQuarks, _ := currencycreator.EstimateSell(&currencycreator.EstimateSellArgs{
			SellAmountInQuarks:   proto.Quarks,
			CurrentValueInQuarks: reserveRecord.CoreMintLocked,
			ValueMintDecimals:    uint8(common.CoreMintDecimals),
			SellFeeBps:           0,
		})

		// Given the sell value, does it align with the native amount in the target currency
		// within half a penny?
		nativeAmountLowerBound := new(big.Float).Sub(clientNativeAmount, nativeAmountErrorThreshold)
		if nativeAmountLowerBound.Cmp(nativeAmountErrorThreshold) < 0 {
			nativeAmountLowerBound = nativeAmountErrorThreshold
		}
		nativeAmountUpperBound := new(big.Float).Add(clientNativeAmount, nativeAmountErrorThreshold)
		coreMintSellValueInUnits := new(big.Float).Quo(
			big.NewFloat(float64(coreMintSellValueInQuarks)).SetPrec(defaultPrecision),
			big.NewFloat(float64(coreMintQuarksPerUnit)).SetPrec(defaultPrecision),
		)
		potentialNativeAmount := new(big.Float).Mul(new(big.Float).Quo(otherRate, usdRate), coreMintSellValueInUnits)
		if potentialNativeAmount.Cmp(nativeAmountLowerBound) < 0 || potentialNativeAmount.Cmp(nativeAmountUpperBound) > 0 {
			log.WithFields(logrus.Fields{
				"core_mint_sell_value":      coreMintSellValueInUnits,
				"native_amount_lower_bound": nativeAmountLowerBound,
				"native_amount_upper_bound": nativeAmountUpperBound,
				"potential_native_amount":   potentialNativeAmount,
				"usd_rate":                  usdRate,
				"other_rate":                otherRate,
			}).Info("native amount is outside error threshold")
			continue
		}

		// For the valid native amount, is the exchange rate calculated correctly?
		expectedRate := new(big.Float).Quo(potentialNativeAmount, clientTokenUnits)
		percentDiff := new(big.Float).Quo(new(big.Float).Abs(new(big.Float).Sub(clientRate, expectedRate)), expectedRate)
		if percentDiff.Cmp(rateErrorThreshold) > 0 {
			log.WithField("potential_exchange_rate", expectedRate).Info("exchange rate is outside error threshold")
			continue
		}

		return true, "", nil
	}

	return false, "fiat exchange data is stale or invalid", nil
}
