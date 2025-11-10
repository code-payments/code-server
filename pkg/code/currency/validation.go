package currency

import (
	"context"
	"math"
	"time"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
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

	// Find an exchange rate that the client could have fetched from a RPC call
	// within a reasonable time in the past
	var foundRate float64
	var isClientRateValid bool
	for i := range 2 {
		exchangeRateTime := latestExchangeRateTime.Add(time.Duration(-i) * timePerExchangeRateUpdate)

		exchangeRateRecord, err := data.GetExchangeRate(ctx, currency_lib.Code(proto.Currency), exchangeRateTime)
		if err == currency.ErrNotFound {
			continue
		} else if err != nil {
			return false, "", err
		}

		percentDiff := math.Abs(exchangeRateRecord.Rate-proto.ExchangeRate) / exchangeRateRecord.Rate
		if percentDiff < 0.001 {
			isClientRateValid = true
			foundRate = exchangeRateRecord.Rate
			break
		}
	}

	if !isClientRateValid {
		return false, "fiat exchange rate is stale or invalid", nil
	}

	// Validate that the native amount and exchange rate fall reasonably within
	// the amount of quarks to send in the transaction.
	quarksPerUnit := common.GetMintQuarksPerUnit(common.CoreMintAccount)
	unitsOfCoreMint := proto.NativeAmount / foundRate
	expectedQuarks := int64(unitsOfCoreMint * float64(quarksPerUnit))
	if math.Abs(float64(expectedQuarks-int64(proto.Quarks))) > 1000 {
		return false, "payment native amount and quark value mismatch", nil
	}

	return true, "", nil
}

func validateCurrencyLaunchpadClientExchangeData(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) (bool, string, error) {
	mintAccount, err := common.GetBackwardsCompatMint(proto.Mint)
	if err != nil {
		return false, "", err
	}

	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"currency":      proto.Currency,
		"native_amount": proto.NativeAmount,
		"exhange_rate":  proto.ExchangeRate,
		"quarks":        proto.Quarks,
		"mint":          mintAccount.PublicKey().ToBase58(),
	})

	coreMintQuarksPerUnit := common.GetMintQuarksPerUnit(common.CoreMintAccount)
	otherMintQuarksPerUnit := common.GetMintQuarksPerUnit(mintAccount)

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

		// How much core mint would be received for a sell against the currency creator program?
		coreMintSellValueInQuarks, _ := currencycreator.EstimateSell(&currencycreator.EstimateSellArgs{
			SellAmountInQuarks:   proto.Quarks,
			CurrentValueInQuarks: reserveRecord.CoreMintLocked,
			ValueMintDecimals:    uint8(common.CoreMintDecimals),
			SellFeeBps:           0,
		})

		// Given the sell value, does it align with the native amount in the target currency
		// within half a penny?
		errorThreshold := 0.005
		nativeAmountLowerBound := proto.NativeAmount - errorThreshold
		if nativeAmountLowerBound < errorThreshold {
			nativeAmountLowerBound = errorThreshold
		}
		nativeAmountUpperBound := proto.NativeAmount + errorThreshold
		coreMintSellValueInUnits := float64(coreMintSellValueInQuarks) / float64(coreMintQuarksPerUnit)
		potentialNativeAmount := otherExchangeRateRecord.Rate * coreMintSellValueInUnits / usdExchangeRateRecord.Rate
		if potentialNativeAmount < nativeAmountLowerBound || potentialNativeAmount > nativeAmountUpperBound {
			log.WithFields(logrus.Fields{
				"native_amount_lower_bound": nativeAmountLowerBound,
				"native_amount_upper_bound": nativeAmountUpperBound,
				"potential_native_amount":   potentialNativeAmount,
			}).Info("native amount is outside error threshold")
			continue
		}

		// For the valid native amount, is the exchange rate calculated correctly?
		otherMintUnits := float64(proto.Quarks) / float64(otherMintQuarksPerUnit)
		expectedRate := potentialNativeAmount / otherMintUnits
		percentDiff := math.Abs(proto.ExchangeRate-expectedRate) / expectedRate
		if percentDiff > 0.0001 {
			log.WithField("potential_exchange_rate", expectedRate).Info("exchange rate is outside error threshold")
			continue
		}

		return true, "", nil
	}

	return false, "fiat exchange data is stale or invalid", nil
}
