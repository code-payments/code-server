package chat

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendMerchantExchangeMessage sends a message to the merchant's chat with
// exchange data content related to the submitted intent. Intents that
// don't belong in the merchant chat will be ignored.
//
// Note: Tests covered in SubmitIntent history tests
func SendMerchantExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) error {
	messageId := intentRecord.IntentId

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
	isVerifiedChat := false

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil
	}

	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
			paymentRequestRecord, err := data.GetPaymentRequest(ctx, intentRecord.IntentId)
			if err != nil {
				return errors.Wrap(err, "error getting payment request record")
			}

			if paymentRequestRecord.Domain != nil {
				chatTitle = *paymentRequestRecord.Domain
				chatType = chat.ChatTypeExternalApp
				isVerifiedChat = paymentRequestRecord.IsVerified
			}

			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_SPENT
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_PAID
			}
		} else if intentRecord.SendPrivatePaymentMetadata.IsWithdrawal {
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return err
				} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts only exist against verified merchants,
					// and will have merchant payments appear in the verified merchant
					// chat.
					chatTitle = *destinationAccountInfoRecord.RelationshipTo
					chatType = chat.ChatTypeExternalApp
					isVerifiedChat = true
					verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
				}
			}
		}
	case intent.SendPublicPayment:
		if intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
			if len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return err
				} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts only exist against verified merchants,
					// and will have merchant payments appear in the verified merchant
					// chat.
					chatTitle = *destinationAccountInfoRecord.RelationshipTo
					chatType = chat.ChatTypeExternalApp
					isVerifiedChat = true
					verbByMessageReceiver[intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
				}
			}
		}
	case intent.ExternalDeposit:
		messageId = strings.Split(messageId, "-")[0]
		destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.ExternalDepositMetadata.DestinationTokenAccount)
		if err != nil {
			return err
		} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
			// Relationship accounts only exist against verified merchants,
			// and will have merchant payments appear in the verified merchant
			// chat.
			chatTitle = *destinationAccountInfoRecord.RelationshipTo
			chatType = chat.ChatTypeExternalApp
			isVerifiedChat = true
			verbByMessageReceiver[intentRecord.ExternalDepositMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
		}
	default:
		return nil
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
			isVerifiedChat,
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
