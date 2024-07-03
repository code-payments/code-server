package chat

import (
	"context"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/localization"
)

// SendCodeTeamMessage sends a message to the Code Team chat.
func SendCodeTeamMessage(ctx context.Context, data code_data.Provider, receiver *common.Account, chatMessage *chatpb.ChatMessage) (bool, error) {
	return SendChatMessage(
		ctx,
		data,
		CodeTeamName,
		chat_v1.ChatTypeInternal,
		true,
		receiver,
		chatMessage,
		false,
	)
}

// ToWelcomeBonusMessage turns the intent record into a welcome bonus chat message
// to be inserted into the Code Team chat.
func ToWelcomeBonusMessage(intentRecord *intent.Record) (*chatpb.ChatMessage, error) {
	return newIncentiveMessage(localization.ChatMessageWelcomeBonus, intentRecord)
}

// ToReferralBonusMessage turns the intent record into a referral bonus chat message
// to be inserted into the Code Team chat.
func ToReferralBonusMessage(intentRecord *intent.Record) (*chatpb.ChatMessage, error) {
	return newIncentiveMessage(localization.ChatMessageReferralBonus, intentRecord)
}

func newIncentiveMessage(localizedTextKey string, intentRecord *intent.Record) (*chatpb.ChatMessage, error) {
	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil, errors.New("exchange data not available")
	}

	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_ServerLocalized{
				ServerLocalized: &chatpb.ServerLocalizedContent{
					KeyOrText: localizedTextKey,
				},
			},
		},
		{
			Type: &chatpb.Content_ExchangeData{
				ExchangeData: &chatpb.ExchangeDataContent{
					Verb: chatpb.ExchangeDataContent_RECEIVED,
					ExchangeData: &chatpb.ExchangeDataContent_Exact{
						Exact: exchangeData,
					},
				},
			},
		},
	}

	return newProtoChatMessage(intentRecord.IntentId, content, intentRecord.CreatedAt)
}
