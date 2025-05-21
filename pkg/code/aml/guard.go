package aml

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/limit"
	currency_util "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/metrics"
)

var (
	// These limits are intentionally higher than that enforced on clients,
	// so we can do better rounding on limits per currency.
	//
	// todo: configurable
	maxDailyUsdLimit = 1.2 * limit.SendLimits[currency_util.USD].Daily
)

// Guard gates money movement by applying rules on operations of interest to
// discourage money laundering.
type Guard struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewGuard(data code_data.Provider) *Guard {
	return &Guard{
		log:  logrus.StandardLogger().WithField("type", "aml/guard"),
		data: data,
	}
}

// AllowMoneyMovement determines whether an intent that moves funds is allowed
// to be executed.
func (g *Guard) AllowMoneyMovement(ctx context.Context, intentRecord *intent.Record) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowMoneyMovement")
	defer tracer.End()

	var currency currency_util.Code
	var nativeAmount float64
	var usdMarketValue float64
	var consumptionCalculator func(ctx context.Context, owner string, since time.Time) (uint64, float64, error)
	switch intentRecord.IntentType {
	case intent.SendPublicPayment:
		// Public sends are subject to limits
		currency = intentRecord.SendPublicPaymentMetadata.ExchangeCurrency
		nativeAmount = intentRecord.SendPublicPaymentMetadata.NativeAmount
		usdMarketValue = intentRecord.SendPublicPaymentMetadata.UsdMarketValue
		consumptionCalculator = g.data.GetTransactedAmountForAntiMoneyLaundering
	case intent.ReceivePaymentsPublicly:
		// Public receives are always allowed
		return true, nil
	default:
		err := errors.New("intent record must be a send or receive payment")
		tracer.OnError(err)
		return false, err
	}

	log := g.log.WithFields(logrus.Fields{
		"method":        "AllowMoneyMovement",
		"owner":         intentRecord.InitiatorOwnerAccount,
		"currency":      string(currency),
		"native_amount": nativeAmount,
		"usd_value":     usdMarketValue,
	})

	sendLimit, ok := limit.SendLimits[currency]
	if !ok {
		log.Info("denying intent with unsupported currency")
		recordDenialEvent(ctx, "unsupported currency")
		return false, nil
	}

	if nativeAmount > sendLimit.PerTransaction {
		log.Info("denying intent that exceeds per-transaction value")
		recordDenialEvent(ctx, "exceeds per-transaction value")
		return false, nil
	}

	// Bound the maximum dollar value of payments in the last day
	_, usdInLastDay, err := consumptionCalculator(ctx, intentRecord.InitiatorOwnerAccount, time.Now().Add(-24*time.Hour))
	if err != nil {
		log.WithError(err).Warn("failure calculating previous day transaction amount")
		tracer.OnError(err)
		return false, err
	}

	if usdInLastDay+usdMarketValue > maxDailyUsdLimit {
		log.Info("denying intent that exceeds daily usd limit")
		recordDenialEvent(ctx, "exceeds daily usd value")
		return false, nil
	}

	return true, nil
}
