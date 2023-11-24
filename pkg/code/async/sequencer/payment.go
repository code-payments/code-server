package async_sequencer

import (
	"context"
	"errors"
	"time"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

func savePaymentRecord(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	var transactionIndex uint32
	switch fulfillmentRecord.FulfillmentType {
	case fulfillment.NoPrivacyWithdraw:
		transactionIndex = 4
	case fulfillment.TemporaryPrivacyTransferWithAuthority,
		fulfillment.PermanentPrivacyTransferWithAuthority,
		fulfillment.TransferWithCommitment,
		fulfillment.NoPrivacyTransferWithAuthority:
		transactionIndex = 2
	default:
		return errors.New("cannot save payment for fulfillment type")
	}

	actionRecord, err := data.GetActionById(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return err
	}

	if txnRecord.HasErrors || txnRecord.ConfirmationState == transaction.ConfirmationFailed {
		return errors.New("cannot save a failed payment")
	}

	if txnRecord.ConfirmationState != transaction.ConfirmationFinalized {
		return errors.New("cannot save a payment that hasn't finalized")
	}

	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
	if err != nil {
		return err
	}

	paymentRecord := &payment.Record{
		// Source and destination should come from fulfillment, since intent or action
		// don't include the intermediary steps between accounts that aren't user accounts.
		Source:      fulfillmentRecord.Source,
		Destination: *fulfillmentRecord.Destination,

		// todo: Assumes we don't split payments across multiple transactions in an action
		Quantity: *actionRecord.Quantity,

		// todo: Just filling this in with KIN currency and latest USD rate. I don't think
		//       these make sense in payment records anymore. These details are captured by
		//       intents.
		ExchangeCurrency: string(currency_lib.KIN),
		ExchangeRate:     1.0,
		UsdMarketValue:   usdExchangeRecord.Rate * float64(kin.FromQuarks(*actionRecord.Quantity)),

		IsExternal: false,
		Rendezvous: fulfillmentRecord.Intent,

		TransactionId:    txnRecord.Signature,
		TransactionIndex: transactionIndex,
		BlockId:          txnRecord.Slot,
		BlockTime:        txnRecord.BlockTime,

		ConfirmationState: transaction.ConfirmationFinalized,

		CreatedAt: time.Now(),
	}
	return data.UpdateOrCreatePayment(ctx, paymentRecord)
}
