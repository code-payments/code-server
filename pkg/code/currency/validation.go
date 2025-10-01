package currency

import (
	"context"
	"math"
	"time"

	"github.com/pkg/errors"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
)

// GetPotentialClientCoreMintExchangeRates gets a set of fiat exchange rates that
// a client attempting to maintain a latest state could have fetched from the
// currency server for the core mint.
func GetPotentialClientCoreMintExchangeRates(ctx context.Context, data code_data.Provider, code currency_lib.Code) ([]*currency.ExchangeRateRecord, error) {
	exchangeRecords, err := data.GetExchangeRateHistory(
		ctx,
		code,
		query.WithStartTime(time.Now().Add(-30*time.Minute)), // Give enough leeway to allow for 15 minute old rates
		query.WithEndTime(time.Now().Add(time.Minute)),       // Ensure we pick up recent exchange rates
		query.WithLimit(32),                   // Try to pick up all records
		query.WithDirection(query.Descending), // Optimize for most recent records first
	)
	if err != nil && err != currency.ErrNotFound {
		return nil, err
	}

	// To handle cases where the exchange rate worker might be down, try
	// loading the latest rate as clients would have and add it to valid
	// set of comparable records.
	latestExchangeRecord, err := data.GetExchangeRate(
		ctx,
		code,
		GetLatestExchangeRateTime(),
	)
	if err != nil && err != currency.ErrNotFound {
		return nil, err
	}

	if latestExchangeRecord != nil {
		exchangeRecords = append(exchangeRecords, latestExchangeRecord)
	}

	// This is bad, and means we can't query for any records
	if len(exchangeRecords) == 0 {
		return nil, errors.Errorf("found no exchange records for %s currency", code)
	}
	return exchangeRecords, nil
}

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

		// Given the sell value, does it align with the native amount in the target currency?
		coreMintSellValueInUnits := float64(coreMintSellValueInQuarks) / float64(coreMintQuarksPerUnit)
		potentialNativeAmount := otherExchangeRateRecord.Rate * coreMintSellValueInUnits / usdExchangeRateRecord.Rate
		percentDiff := math.Abs(proto.NativeAmount-potentialNativeAmount) / potentialNativeAmount
		if percentDiff > 0.001 {
			continue
		}

		// For the valid native amount, is the exchange rate calculated correctly?
		otherMintUnits := float64(proto.Quarks) / float64(otherMintQuarksPerUnit)
		expectedRate := potentialNativeAmount / otherMintUnits
		percentDiff = math.Abs(proto.ExchangeRate-expectedRate) / expectedRate
		if percentDiff > 0.001 {
			continue
		}

		return true, "", nil
	}

	return false, "fiat exchange data is stale or invalid", nil
}
