package chat

import (
	"context"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendTipsExchangeMessage sends a message to the Tips chat with exchange data
// content related to the submitted intent. Intents that don't belong in the
// Tips chat will be ignored.
//
// Note: Tests covered in SubmitIntent history tests
func SendTipsExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) ([]*MessageWithOwner, error) {
	messageId := intentRecord.IntentId

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil, nil
	}

	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if !intentRecord.SendPrivatePaymentMetadata.IsTip {
			// Not a tip
			return nil, nil
		}

		verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_SENT_TIP
		if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
			verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_RECEIVED_TIP
		}
	default:
		return nil, nil
	}

	var messagesToPush []*MessageWithOwner
	for account, verb := range verbByMessageReceiver {
		receiver, err := common.NewAccountFromPublicKeyString(account)
		if err != nil {
			return nil, err
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
			return nil, errors.Wrap(err, "error creating proto chat message")
		}

		canPush, err := SendChatMessage(
			ctx,
			data,
			TipsName,
			chat_v1.ChatTypeInternal,
			true,
			receiver,
			protoMessage,
			verb != chatpb.ExchangeDataContent_RECEIVED_TIP,
		)
		if err != nil && err != chat_v1.ErrMessageAlreadyExists {
			return nil, errors.Wrap(err, "error persisting chat message")
		}

		if canPush {
			messagesToPush = append(messagesToPush, &MessageWithOwner{
				Owner:   receiver,
				Title:   TipsName,
				Message: protoMessage,
			})
		}
	}

	return messagesToPush, nil
}
