package chat

import (
	"context"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendMerchantExchangeMessage sends a message to the merchant's chat with
// exchange data content related to the submitted intent.
//
// Note: Tests covered in SubmitIntent history tests
func SendMerchantExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) error {
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if !intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
			return nil
		}
	default:
		return nil
	}

	paymentRequestRecord, err := data.GetPaymentRequest(ctx, intentRecord.IntentId)
	if err != nil {
		return errors.Wrap(err, "error getting payment request record")
	}

	// There are three possible chats for a merchant:
	//  1. Verified chat with a verified identifier that server has validated
	//  2. Unverified chat with an unverified identifier
	//  3. Fallback internal "Payments" chat when no identifier is provided
	// These chats represent upgrades in functionality. From 2 to 3, we can enable
	// an identifier. From 2 to 1, we can enable messaging from the merchant to the
	// user, and guarantee the chat is clean with only messages originiating from
	// the merchant. Representation in the UI may differ (ie. 2 and 3 are grouped),
	// but this is the most flexible solution with the chat model.
	chatTitle := PaymentsName
	chatType := chat.ChatTypeInternal
	isVerified := false
	if paymentRequestRecord.Domain != nil {
		chatTitle = *paymentRequestRecord.Domain
		chatType = chat.ChatTypeExternalApp
		isVerified = paymentRequestRecord.IsVerified
	}

	messageId := intentRecord.IntentId

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil
	}

	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_SPENT
		if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
			verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_PAID
		}
	}

	for account, verb := range verbByMessageReceiver {
		receiver, err := common.NewAccountFromPublicKeyString(account)
		if err != nil {
			return err
		}

		content := []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: verb,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: exchangeData,
						},
					},
				},
			},
		}
		protoMessage, err := newProtoChatMessage(messageId, content, intentRecord.CreatedAt)
		if err != nil {
			return errors.Wrap(err, "error creating proto chat message")
		}

		_, err = SendChatMessage(
			ctx,
			data,
			chatTitle,
			chatType,
			isVerified,
			receiver,
			protoMessage,
			true,
		)
		if err != nil && err != chat.ErrMessageAlreadyExists {
			return errors.Wrap(err, "error persisting chat message")
		}
	}
	return nil
}
