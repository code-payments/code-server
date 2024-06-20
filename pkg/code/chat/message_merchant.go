package chat

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendMerchantExchangeMessage sends a message to the merchant's chat with
// exchange data content related to the submitted intent. Intents that
// don't belong in the merchant chat will be ignored. The set of chat messages
// that should be pushed are returned.
//
// Note: Tests covered in SubmitIntent history tests
func SendMerchantExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record, actionRecords []*action.Record) ([]*MessageWithOwner, error) {
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
	chatType := chat_v1.ChatTypeInternal
	isVerifiedChat := false

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil, nil
	}

	type verbAndExchangeData struct {
		verb         chatpb.ExchangeDataContent_Verb
		exchangeData *transactionpb.ExchangeData
	}
	verbAndExchangeDataByMessageReceiver := make(map[string]*verbAndExchangeData)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
			paymentRequestRecord, err := data.GetRequest(ctx, intentRecord.IntentId)
			if err != nil {
				return nil, errors.Wrap(err, "error getting request record")
			}

			if paymentRequestRecord.Domain != nil {
				chatTitle = *paymentRequestRecord.Domain
				chatType = chat_v1.ChatTypeExternalApp
				isVerifiedChat = paymentRequestRecord.IsVerified
			}

			verbAndExchangeDataByMessageReceiver[intentRecord.InitiatorOwnerAccount] = &verbAndExchangeData{
				verb:         chatpb.ExchangeDataContent_SPENT,
				exchangeData: exchangeData,
			}
			receiveByOwner, err := getMicroPaymentReceiveExchangeDataByOwner(ctx, data, exchangeData, intentRecord, actionRecords)
			if err != nil {
				return nil, err
			}
			for owner, exchangeData := range receiveByOwner {
				verbAndExchangeDataByMessageReceiver[owner] = &verbAndExchangeData{
					verb:         chatpb.ExchangeDataContent_RECEIVED,
					exchangeData: exchangeData,
				}
			}
		} else if intentRecord.SendPrivatePaymentMetadata.IsWithdrawal {
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return nil, err
				} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts only exist against verified merchants,
					// and will have merchant payments appear in the verified merchant
					// chat.
					chatTitle = *destinationAccountInfoRecord.RelationshipTo
					chatType = chat_v1.ChatTypeExternalApp
					isVerifiedChat = true
					verbAndExchangeDataByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = &verbAndExchangeData{
						verb:         chatpb.ExchangeDataContent_RECEIVED,
						exchangeData: exchangeData,
					}
				}
			}
		}
	case intent.SendPublicPayment:
		if intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
			if len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return nil, err
				} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts only exist against verified merchants,
					// and will have merchant payments appear in the verified merchant
					// chat.
					chatTitle = *destinationAccountInfoRecord.RelationshipTo
					chatType = chat_v1.ChatTypeExternalApp
					isVerifiedChat = true
					verbAndExchangeDataByMessageReceiver[intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount] = &verbAndExchangeData{
						verb:         chatpb.ExchangeDataContent_RECEIVED,
						exchangeData: exchangeData,
					}
				}
			}
		}
	case intent.ExternalDeposit:
		messageId = strings.Split(messageId, "-")[0]
		destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.ExternalDepositMetadata.DestinationTokenAccount)
		if err != nil {
			return nil, err
		} else if destinationAccountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
			// Relationship accounts only exist against verified merchants,
			// and will have merchant payments appear in the verified merchant
			// chat.
			chatTitle = *destinationAccountInfoRecord.RelationshipTo
			chatType = chat_v1.ChatTypeExternalApp
			isVerifiedChat = true
			verbAndExchangeDataByMessageReceiver[intentRecord.ExternalDepositMetadata.DestinationOwnerAccount] = &verbAndExchangeData{
				verb:         chatpb.ExchangeDataContent_RECEIVED,
				exchangeData: exchangeData,
			}
		}
	default:
		return nil, nil
	}

	var messagesToPush []*MessageWithOwner
	for account, verbAndExchangeData := range verbAndExchangeDataByMessageReceiver {
		receiver, err := common.NewAccountFromPublicKeyString(account)
		if err != nil {
			return nil, err
		}

		content := []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: verbAndExchangeData.verb,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: verbAndExchangeData.exchangeData,
						},
					},
				},
			},
		}
		protoMessage, err := newProtoChatMessage(messageId, content, intentRecord.CreatedAt)
		if err != nil {
			return nil, errors.Wrap(err, "error creating proto chat message")
		}

		canPush, err := SendChatMessage(
			ctx,
			data,
			chatTitle,
			chatType,
			isVerifiedChat,
			receiver,
			protoMessage,
			verbAndExchangeData.verb != chatpb.ExchangeDataContent_RECEIVED || !isVerifiedChat,
		)
		if err != nil && err != chat_v1.ErrMessageAlreadyExists {
			return nil, errors.Wrap(err, "error persisting chat message")
		}

		if canPush {
			messagesToPush = append(messagesToPush, &MessageWithOwner{
				Owner:   receiver,
				Title:   chatTitle,
				Message: protoMessage,
			})
		}
	}
	return messagesToPush, nil
}
