package currency

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pkg/errors"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
)

// todo: add tests, but generally well tested in server tests since that's where most of this originated

var (
	SmallestSendAmount = common.CoreMintQuarksPerUnit / 100
)

// GetPotentialClientExchangeRates gets a set of exchange rates that a client
// attempting to maintain a latest state could have fetched from the currency
// server.
func GetPotentialClientExchangeRates(ctx context.Context, data code_data.Provider, code currency_lib.Code) ([]*currency.ExchangeRateRecord, error) {
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
	currencyCode := strings.ToLower(proto.Currency)
	switch currencyCode {
	case string(common.CoreMintSymbol):
		if proto.ExchangeRate != 1.0 {
			return false, "core mint exchange rate must be 1", nil
		}
	default:
		// Validate the exchange rate with what Code would have returned
		exchangeRecords, err := GetPotentialClientExchangeRates(ctx, data, currency_lib.Code(currencyCode))
		if err != nil {
			return false, "", err
		}

		// Alternatively, we could find the highest and lowest value and ensure
		// the requested rate falls in that range. However, this method allows
		// us to ensure clients are getting their data from code-server.
		var foundExchangeRate bool
		for _, exchangeRecord := range exchangeRecords {
			// Avoid issues with floating points by examining the percentage
			// difference
			percentDiff := math.Abs(exchangeRecord.Rate-proto.ExchangeRate) / exchangeRecord.Rate
			if percentDiff < 0.001 {
				foundExchangeRate = true
				break
			}
		}

		if !foundExchangeRate {
			return false, "fiat exchange rate is stale", nil
		}
	}

	// Validate that the native amount and exchange rate fall reasonably within
	// the amount of quarks to send in the transaction. This must consider any
	// truncation at the client due to minimum bucket sizes.
	//
	// todo: This uses string conversions, which is less than ideal, but the only
	//       thing available at the time of writing this for conversion.
	quarksFromCurrency, _ := common.StrToQuarks(fmt.Sprintf("%.6f", proto.NativeAmount/proto.ExchangeRate))
	if math.Abs(float64(quarksFromCurrency-int64(proto.Quarks))) > float64(SmallestSendAmount) {
		return false, "payment native amount and quark value mismatch", nil
	}

	return true, "", nil
}
