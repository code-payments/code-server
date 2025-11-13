package currency

import (
	"context"
	"math"
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

	coreMintQuarksPerUnit := common.GetMintQuarksPerUnit(common.CoreMintAccount)

	clientRate := big.NewFloat(proto.ExchangeRate).SetPrec(defaultPrecision)
	clientNativeAmount := big.NewFloat(proto.NativeAmount).SetPrec(defaultPrecision)
	clientQuarks := big.NewFloat(float64(proto.Quarks)).SetPrec(defaultPrecision)

	currencyDecimals := currency_lib.GetDecimals(currency_lib.Code(proto.Currency))
	one := big.NewFloat(1.0).SetPrec(defaultPrecision)
	minTransferValue := new(big.Float).Quo(one, big.NewFloat(math.Pow10(currencyDecimals)))

	rateErrorThreshold := big.NewFloat(0.001).SetPrec(defaultPrecision)
	nativeAmountErrorThreshold := new(big.Float).Quo(minTransferValue, big.NewFloat(2.0))

	nativeAmountLowerBound := new(big.Float).Sub(clientNativeAmount, nativeAmountErrorThreshold)
	if nativeAmountLowerBound.Cmp(nativeAmountErrorThreshold) < 0 {
		nativeAmountLowerBound = nativeAmountErrorThreshold
	}
	nativeAmountUpperBound := new(big.Float).Add(clientNativeAmount, nativeAmountErrorThreshold)
	quarksLowerBound := new(big.Float).Mul(new(big.Float).Quo(nativeAmountLowerBound, clientRate), big.NewFloat(float64(coreMintQuarksPerUnit)))
	quarksUpperBound := new(big.Float).Mul(new(big.Float).Quo(nativeAmountUpperBound, clientRate), big.NewFloat(float64(coreMintQuarksPerUnit)))

	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"currency":                  proto.Currency,
		"client_native_amount":      clientNativeAmount,
		"client_exchange_rate":      clientRate,
		"client_quarks":             proto.Quarks,
		"min_transfer_value":        minTransferValue,
		"native_amount_lower_bound": nativeAmountLowerBound,
		"native_amount_upper_bound": nativeAmountUpperBound,
		"quarks_lower_bound":        quarksLowerBound,
		"quarks_upper_bound":        quarksUpperBound,
	})

	if clientNativeAmount.Cmp(nativeAmountErrorThreshold) < 0 {
		log.Info("native amount is less than minimum transfer value error threshold")
		return false, "native amount is less than minimum transfer value error threshold", nil
	}

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

		log.WithField("rate", actualRate).Info("exchange rate doesn't match")
	}

	if !isClientRateValid {
		log.Info("fiat exchange rate is stale or invalid")
		return false, "fiat exchange rate is stale or invalid", nil
	}

	// Validate that the native amount within half of the minimum transfer value
	if clientQuarks.Cmp(quarksLowerBound) < 0 || clientQuarks.Cmp(quarksUpperBound) > 0 {
		log.Info("native amount is outside error threshold")
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

	currencyDecimals := currency_lib.GetDecimals(currency_lib.Code(proto.Currency))
	one := big.NewFloat(1.0).SetPrec(defaultPrecision)
	minTransferValue := new(big.Float).Quo(one, big.NewFloat(math.Pow10(currencyDecimals)))

	rateErrorThreshold := big.NewFloat(0.001).SetPrec(defaultPrecision)
	nativeAmountErrorThreshold := new(big.Float).Quo(minTransferValue, big.NewFloat(2.0))

	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"currency":             proto.Currency,
		"client_native_amount": clientNativeAmount,
		"client_exchange_rate": clientRate,
		"client_token_units":   clientTokenUnits,
		"client_quarks":        proto.Quarks,
		"mint":                 mintAccount.PublicKey().ToBase58(),
		"min_transfer_value":   minTransferValue,
	})

	if clientNativeAmount.Cmp(nativeAmountErrorThreshold) < 0 {
		log.Info("native amount is less than minimum transfer value error threshold")
		return false, "native amount is less than minimum transfer value error threshold", nil
	}

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
		// within half a minimum transfer unit?
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

		log := log.WithFields(logrus.Fields{
			"core_mint_sell_value":      coreMintSellValueInUnits,
			"native_amount_lower_bound": nativeAmountLowerBound,
			"native_amount_upper_bound": nativeAmountUpperBound,
			"potential_native_amount":   potentialNativeAmount,
			"usd_rate":                  usdRate,
			"other_rate":                otherRate,
		})

		if potentialNativeAmount.Cmp(nativeAmountLowerBound) < 0 || potentialNativeAmount.Cmp(nativeAmountUpperBound) > 0 {
			log.Info("native amount is outside error threshold")
			continue
		}

		// For the valid native amount, is the exchange rate calculated correctly?
		expectedRate := new(big.Float).Quo(clientNativeAmount, clientTokenUnits)
		percentDiff := new(big.Float).Quo(new(big.Float).Abs(new(big.Float).Sub(clientRate, expectedRate)), expectedRate)

		log = log.WithField("potential_exchange_rate", expectedRate)

		if percentDiff.Cmp(rateErrorThreshold) > 0 {
			log.Info("exchange rate is outside error threshold")
			continue
		}

		return true, "", nil
	}

	return false, "fiat exchange data is stale or invalid", nil
}
