package chat

import (
	"context"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
)

func newProtoChatMessage(
	messageId string,
	content []*chatpb.Content,
	ts time.Time,
) (*chatpb.ChatMessage, error) {
	decodedMessageId, err := base58.Decode(messageId)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding message id")
	}

	if len(decodedMessageId) != 32 && len(decodedMessageId) != 64 {
		return nil, errors.Errorf("invalid message id length of %d", len(decodedMessageId))
	}

	msg := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: decodedMessageId,
		},
		Ts:      timestamppb.New(ts),
		Content: content,
	}

	if err := msg.Validate(); err != nil {
		return nil, errors.Wrap(err, "chat message failed validation")
	}

	return msg, nil
}

// todo: promote more broadly?
func getExchangeDataFromIntent(intentRecord *intent.Record) (*transactionpb.ExchangeData, bool) {
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		return &transactionpb.ExchangeData{
			Currency:     string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency),
			ExchangeRate: intentRecord.SendPrivatePaymentMetadata.ExchangeRate,
			NativeAmount: intentRecord.SendPrivatePaymentMetadata.NativeAmount,
			Quarks:       intentRecord.SendPrivatePaymentMetadata.Quantity,
		}, true
	case intent.SendPublicPayment:
		return &transactionpb.ExchangeData{
			Currency:     string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency),
			ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
			NativeAmount: intentRecord.SendPublicPaymentMetadata.NativeAmount,
			Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
		}, true
	case intent.ReceivePaymentsPrivately:
		return &transactionpb.ExchangeData{
			Currency:     string(currency_lib.KIN),
			ExchangeRate: 1.0,
			NativeAmount: float64(intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity) / kin.QuarksPerKin,
			Quarks:       intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity,
		}, true
	case intent.ReceivePaymentsPublicly:
		return &transactionpb.ExchangeData{
			Currency:     string(intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency),
			ExchangeRate: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate,
			NativeAmount: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount,
			Quarks:       intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
		}, true
	case intent.MigrateToPrivacy2022:
		return &transactionpb.ExchangeData{
			Currency:     string(currency_lib.KIN),
			ExchangeRate: 1.0,
			NativeAmount: float64(intentRecord.MigrateToPrivacy2022Metadata.Quantity) / kin.QuarksPerKin,
			Quarks:       intentRecord.MigrateToPrivacy2022Metadata.Quantity,
		}, true
	case intent.ExternalDeposit:
		return &transactionpb.ExchangeData{
			Currency:     string(currency_lib.KIN),
			ExchangeRate: 1.0,
			NativeAmount: float64(intentRecord.ExternalDepositMetadata.Quantity) / kin.QuarksPerKin,
			Quarks:       intentRecord.ExternalDepositMetadata.Quantity,
		}, true
	}

	return nil, false
}

func getMicroPaymentReceiveExchangeDataByOwner(
	ctx context.Context,
	data code_data.Provider,
	exchangeData *transactionpb.ExchangeData,
	intentRecord *intent.Record,
	actionRecords []*action.Record,
) (map[string]*transactionpb.ExchangeData, error) {
	if intentRecord.IntentType != intent.SendPrivatePayment || !intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		return nil, errors.New("intent is not a micro payment")
	}

	// Find the action record where the final payment is made
	var thirdPartyPaymentAction *action.Record
	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType != action.NoPrivacyWithdraw {
			continue
		}

		if *actionRecord.Destination == intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount {
			thirdPartyPaymentAction = actionRecord
			break
		}
	}

	// Should never happen if the intent is a micropayment
	if thirdPartyPaymentAction == nil {
		return nil, errors.New("payment action is missing")
	}

	quarksByTokenAccount := make(map[string]uint64)
	quarksByTokenAccount[*thirdPartyPaymentAction.Destination] = *thirdPartyPaymentAction.Quantity

	// Find and consolidate all fee payments into a quark amount by token account
	var foundCodeFee bool
	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType != action.NoPrivacyTransfer {
			continue
		}

		if actionRecord.Source != thirdPartyPaymentAction.Source {
			continue
		}

		// The first fee is always Code, and can be skipped
		if !foundCodeFee {
			foundCodeFee = true
			continue
		}

		quarksByTokenAccount[*actionRecord.Destination] += *actionRecord.Quantity
	}

	// Consolidate quark amount by owner account
	quarksByOwnerAccount := make(map[string]uint64)
	for tokenAccount, quarks := range quarksByTokenAccount {
		if tokenAccount == intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount {
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				quarksByOwnerAccount[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] += quarks
			}
			continue
		}

		accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, tokenAccount)
		if err == nil {
			quarksByOwnerAccount[accountInfoRecord.OwnerAccount] += quarks
		} else if err != account.ErrAccountInfoNotFound {
			return nil, err
		}
	}

	// Map result to an exchange data
	res := make(map[string]*transactionpb.ExchangeData)
	for ownerAccount, quarks := range quarksByOwnerAccount {
		res[ownerAccount] = getExchangeDataInOtherQuarkAmount(exchangeData, quarks)
	}
	return res, nil
}

func getExchangeDataInOtherQuarkAmount(original *transactionpb.ExchangeData, quarks uint64) *transactionpb.ExchangeData {
	nativeAmount := original.NativeAmount
	if original.Quarks != quarks {
		nativeAmount = original.ExchangeRate * float64(quarks) / float64(kin.QuarksPerKin)
	}

	return &transactionpb.ExchangeData{
		Currency:     original.Currency,
		ExchangeRate: original.ExchangeRate,
		NativeAmount: nativeAmount,
		Quarks:       quarks,
	}
}
