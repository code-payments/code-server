package chat

import (
	"context"
	"fmt"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	chatv2pb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	chat_v2 "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendTipsExchangeMessage sends a message to the Tips chat with exchange data
// content related to the submitted intent. Intents that don't belong in the
// Tips chat will be ignored.
//
// Note: Tests covered in SubmitIntent history tests
func SendTipsExchangeMessage(ctx context.Context, data code_data.Provider, notifier Notifier, intentRecord *intent.Record) ([]*MessageWithOwner, error) {
	intentIdRaw, err := base58.Decode(intentRecord.IntentId)
	if err != nil {
		return nil, fmt.Errorf("invalid intent id: %w", err)
	}

	messageId := intentRecord.IntentId

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil, nil
	}

	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if !intentRecord.SendPrivatePaymentMetadata.IsTip {
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

		v1Message, err := newProtoChatMessage(messageId, content, intentRecord.CreatedAt)
		if err != nil {
			return nil, errors.Wrap(err, "error creating proto chat message")
		}

		v2Message := &chatv2pb.ChatMessage{
			MessageId: chat_v2.GenerateMessageId().ToProto(),
			Content: []*chatv2pb.Content{
				{
					Type: &chatv2pb.Content_ExchangeData{
						ExchangeData: &chatv2pb.ExchangeDataContent{
							Verb: chatv2pb.ExchangeDataContent_Verb(verb),
							ExchangeData: &chatv2pb.ExchangeDataContent_Exact{
								Exact: exchangeData,
							},
							Reference: &chatv2pb.ExchangeDataContent_Intent{
								Intent: &commonpb.IntentId{Value: intentIdRaw},
							},
						},
					},
				},
			},
			Ts: timestamppb.New(intentRecord.CreatedAt),
		}

		canPush, err := SendNotificationChatMessageV1(
			ctx,
			data,
			TipsName,
			chat_v1.ChatTypeInternal,
			true,
			receiver,
			v1Message,
			verb != chatpb.ExchangeDataContent_RECEIVED_TIP,
		)
		if err != nil && !errors.Is(err, chat_v1.ErrMessageAlreadyExists) {
			return nil, errors.Wrap(err, "error persisting v1 chat message")
		}

		_, err = SendNotificationChatMessageV2(
			ctx,
			data,
			notifier,
			TipsName,
			true,
			receiver,
			v2Message,
			intentRecord.IntentId,
			verb != chatpb.ExchangeDataContent_RECEIVED_TIP,
		)
		if err != nil {
			// TODO: Eventually we'll want to return an error, but for now we'll log
			//       since we're not in 'prod' yet.
			logrus.StandardLogger().WithError(err).Warn("Failed to send notification message (v2)")
		}

		if canPush {
			messagesToPush = append(messagesToPush, &MessageWithOwner{
				Owner:   receiver,
				Title:   TipsName,
				Message: v1Message,
			})
		}
	}

	return messagesToPush, nil
}
