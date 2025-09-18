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

	latestExchangeRateTime := GetLatestExchangeRateTime()

	var foundRate float64
	var isClientRateValid bool
	for i := range 3 {
		exchangeRateTime := latestExchangeRateTime.Add(time.Duration(-i) * timePerExchangeRateUpdate)

		coreMintFiatExchangeRateRecord, err := data.GetExchangeRate(ctx, currency_lib.Code(proto.Currency), exchangeRateTime)
		if err == currency.ErrNotFound {
			continue
		} else if err != nil {
			return false, "", err
		}

		pricePerCoreMint := 1.0
		if mint.PublicKey().ToBase58() != common.CoreMintAccount.PublicKey().ToBase58() {
			reserveRecord, err := data.GetCurrencyReserveAtTime(ctx, mint.PublicKey().ToBase58(), exchangeRateTime)
			if err == currency.ErrNotFound {
				continue
			} else if err != nil {
				return false, "", err
			}

			pricePerCoreMint, _ = currencycreator.EstimateCurrentPrice(reserveRecord.SupplyFromBonding).Float64()
		}

		actualRate := pricePerCoreMint * coreMintFiatExchangeRateRecord.Rate

		// Avoid issues with floating points by examining the percentage difference
		//
		// todo: configurable error tolerance?
		percentDiff := math.Abs(actualRate-proto.ExchangeRate) / actualRate
		if percentDiff < 0.001 {
			isClientRateValid = true
			foundRate = actualRate
			break
		}
	}

	if !isClientRateValid {
		return false, "fiat exchange rate is stale or invalid", nil
	}

	// Validate that the native amount and exchange rate fall reasonably within
	// the amount of quarks to send in the transaction.
	//
	// todo: configurable error tolerance?
	quarksPerUnit := common.GetMintQuarksPerUnit(mint)
	unitsOfMint := proto.NativeAmount / foundRate
	expectedQuarks := int64(unitsOfMint * float64(quarksPerUnit))
	if math.Abs(float64(expectedQuarks-int64(proto.Quarks))) > 100 {
		return false, "payment native amount and quark value mismatch", nil
	}

	return true, "", nil
}
