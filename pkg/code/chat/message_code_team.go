package chat

import (
	"context"
	"time"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/localization"
)

// SendCodeTeamMessage sends a message to the Code Team chat.
func SendCodeTeamMessage(ctx context.Context, data code_data.Provider, receiver *common.Account, chatMessage *chatpb.ChatMessage) (bool, error) {
	return SendChatMessage(
		ctx,
		data,
		CodeTeamName,
		chat.ChatTypeInternal,
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

// ToUsdcDepositedMessage turns details of a USDC deposit transaction into a chat
// message to be inserted into the Code Team chat.
func ToUsdcDepositedMessage(signature string, ts time.Time) (*chatpb.ChatMessage, error) {
	// todo: Don't have a way of propagating quarks, but that's probably ok since
	//       this is a temporary message for testing swaps.
	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_Localized{
				Localized: &chatpb.LocalizedContent{
					KeyOrText: localization.ChatMessageUsdcDeposited,
				},
			},
		},
	}
	return newProtoChatMessage(signature, content, ts)
}

// ToKinAvailableForUseMessage turns details of a USDC swap transaction into a
// chat message to be inserted into the Code Team chat.
func ToKinAvailableForUseMessage(signature string, ts time.Time, purchases ...*transactionpb.ExchangeDataWithoutRate) (*chatpb.ChatMessage, error) {
	if len(purchases) == 0 {
		return nil, errors.New("no purchases for kin available chat message")
	}

	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_Localized{
				Localized: &chatpb.LocalizedContent{
					KeyOrText: localization.ChatMessageKinAvailableForUse,
				},
			},
		},
	}
	for _, purchase := range purchases {
		content = append(content, &chatpb.Content{
			Type: &chatpb.Content_ExchangeData{
				ExchangeData: &chatpb.ExchangeDataContent{
					Verb: chatpb.ExchangeDataContent_PURCHASED,
					ExchangeData: &chatpb.ExchangeDataContent_Partial{
						Partial: purchase,
					},
				},
			},
		})
	}
	return newProtoChatMessage(signature, content, ts)
}

func newIncentiveMessage(localizedTextKey string, intentRecord *intent.Record) (*chatpb.ChatMessage, error) {
	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil, errors.New("exchange data not available")
	}

	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_Localized{
				Localized: &chatpb.LocalizedContent{
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
