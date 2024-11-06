package lawenforcement

import (
	"context"

	"github.com/sirupsen/logrus"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	// These limits are intentionally higher than that enforced on clients,
	// so we can do better rounding on limits per currency.
	//
	// todo: configurable
	maxUsdPrivateBalance   = 500.00  // 2x the 250 USD limit
	maxUsdTransactionValue = 500.00  // 2x the 250 USD limit
	maxDailyUsdLimit       = 1500.00 // 1.5x the 1000 USD limit
)

// AntiMoneyLaunderingGuard gates money movement by applying rules on operations
// of interest to discourage money laundering through Code.
type AntiMoneyLaunderingGuard struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewAntiMoneyLaunderingGuard(data code_data.Provider) *AntiMoneyLaunderingGuard {
	return &AntiMoneyLaunderingGuard{
		log:  logrus.StandardLogger().WithField("type", "aml/guard"),
		data: data,
	}
}

// AllowMoneyMovement determines whether an intent that moves funds is allowed
// to be executed.
func (g *AntiMoneyLaunderingGuard) AllowMoneyMovement(ctx context.Context, intentRecord *intent.Record) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowMoneyMovement")
	defer tracer.End()

	/*
		var usdMarketValue float64
		var consumptionCalculator func(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error)
		switch intentRecord.IntentType {
		case intent.SendPublicPayment, intent.ReceivePaymentsPublicly:
			// Public movements of money are not subject to AML rules. They are
			// done in the open.
			return true, nil
		case intent.SendPrivatePayment:
			usdMarketValue = intentRecord.SendPrivatePaymentMetadata.UsdMarketValue
			consumptionCalculator = g.data.GetTransactedAmountForAntiMoneyLaundering
		case intent.ReceivePaymentsPrivately:
			// Allow users to always receive in-app payments from their temporary incoming
			// accounts. The payment was already allowed when initiatied on the send side.
			if !intentRecord.ReceivePaymentsPrivatelyMetadata.IsDeposit {
				return true, nil
			}

			owner, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
			if err != nil {
				tracer.OnError(err)
				return false, err
			}

			totalPrivateBalance, err := balance.GetPrivateBalance(ctx, g.data, owner)
			if err != nil {
				tracer.OnError(err)
				return false, err
			}

			usdExchangeRecord, err := g.data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
			if err != nil {
				tracer.OnError(err)
				return false, err
			}

			// Do they need the deposit based on total private balance? Note: clients
			// always try to deposit the max as a mechanism of hiding in the crowd, so
			// we can only consider the current balance and whether it makes sense.
			if usdExchangeRecord.Rate*float64(kin.FromQuarks(totalPrivateBalance)) >= maxUsdPrivateBalance {
				recordDenialEvent(ctx, "private balance exceeds threshold")
				return false, nil
			}

			// Otherwise, limit deposits in line with expectations for payments.
			usdMarketValue = intentRecord.ReceivePaymentsPrivatelyMetadata.UsdMarketValue
			consumptionCalculator = g.data.GetDepositedAmountForAntiMoneyLaundering
		default:
			err := errors.New("intent record must be a send or receive payment")
			tracer.OnError(err)
			return false, err
		}

		if intentRecord.InitiatorPhoneNumber == nil {
			err := errors.New("anti-money laundering guard requires an identity")
			tracer.OnError(err)
			return false, err
		}

		log := g.log.WithFields(logrus.Fields{
			"method":       "AllowMoneyMovement",
			"owner":        intentRecord.InitiatorOwnerAccount,
			"phone_number": *intentRecord.InitiatorPhoneNumber,
			"usd_value":    usdMarketValue,
		})

		phoneNumber := *intentRecord.InitiatorPhoneNumber

		// Bound the maximum dollar value of a payment
		if usdMarketValue > maxUsdTransactionValue {
			log.Info("denying intent that exceeds per-transaction usd value")
			recordDenialEvent(ctx, "exceeds per-transaction usd value")
			return false, nil
		}

		// Bound the maximum dollar value of payments in the last day
		_, usdInLastDay, err := consumptionCalculator(ctx, phoneNumber, time.Now().Add(-24*time.Hour))
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
	*/

	return true, nil
}
