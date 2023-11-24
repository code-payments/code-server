package chat

import (
	"crypto/rand"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/code/data/intent"
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

func randomMessageId() string {
	buffer := make([]byte, 32)
	rand.Read(buffer)
	return base58.Encode(buffer)
}
