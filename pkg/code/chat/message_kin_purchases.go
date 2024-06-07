package chat

import (
	"context"
	"time"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/localization"
)

// GetKinPurchasesChatId returns the chat ID for the Kin Purchases chat for a
// given owner account
func GetKinPurchasesChatId(owner *common.Account) chat_v1.ChatId {
	return chat_v1.GetChatId(KinPurchasesName, owner.PublicKey().ToBase58(), true)
}

// SendKinPurchasesMessage sends a message to the Kin Purchases chat.
func SendKinPurchasesMessage(ctx context.Context, data code_data.Provider, receiver *common.Account, chatMessage *chatpb.ChatMessage) (bool, error) {
	return SendChatMessage(
		ctx,
		data,
		KinPurchasesName,
		chat_v1.ChatTypeInternal,
		true,
		receiver,
		chatMessage,
		false,
	)
}

// ToUsdcDepositedMessage turns details of a USDC deposit transaction into a chat
// message to be inserted into the Kin Purchases chat.
func ToUsdcDepositedMessage(signature string, ts time.Time) (*chatpb.ChatMessage, error) {
	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_ServerLocalized{
				ServerLocalized: &chatpb.ServerLocalizedContent{
					KeyOrText: localization.ChatMessageUsdcDeposited,
				},
			},
		},
	}
	return newProtoChatMessage(signature, content, ts)
}

// NewUsdcBeingConvertedMessage generates a new message generated upon initiating
// a USDC swap to be inserted into the Kin Purchases chat.
func NewUsdcBeingConvertedMessage(ts time.Time) (*chatpb.ChatMessage, error) {
	messageId, err := common.NewRandomAccount()
	if err != nil {
		return nil, err
	}

	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_ServerLocalized{
				ServerLocalized: &chatpb.ServerLocalizedContent{
					KeyOrText: localization.ChatMessageUsdcBeingConverted,
				},
			},
		},
	}
	return newProtoChatMessage(messageId.PublicKey().ToBase58(), content, ts)
}

// ToKinAvailableForUseMessage turns details of a USDC swap transaction into a
// chat message to be inserted into the Kin Purchases chat.
func ToKinAvailableForUseMessage(signature string, ts time.Time, purchases ...*transactionpb.ExchangeDataWithoutRate) (*chatpb.ChatMessage, error) {
	if len(purchases) == 0 {
		return nil, errors.New("no purchases for kin available chat message")
	}

	content := []*chatpb.Content{
		{
			Type: &chatpb.Content_ServerLocalized{
				ServerLocalized: &chatpb.ServerLocalizedContent{
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
