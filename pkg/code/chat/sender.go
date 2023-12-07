package chat

import (
	"context"
	"errors"
	"time"

	"github.com/mr-tron/base58"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
)

// SendChatMessage sends a chat message to a receiving owner account.
//
// Note: This function is not responsible for push notifications. This method
// might be called within the context of a DB transaction, which might have
// unrelated failures. A hint as to whether a push should be sent is provided.
func SendChatMessage(
	ctx context.Context,
	data code_data.Provider,
	chatTitle string,
	chatType chat.ChatType,
	isVerifiedChat bool,
	receiver *common.Account,
	protoMessage *chatpb.ChatMessage,
	isSilentMessage bool,
) (canPushMessage bool, err error) {
	chatId := chat.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), isVerifiedChat)

	if protoMessage.Cursor != nil {
		// Let the utilities and GetMessages RPC handle cursors
		return false, errors.New("cursor must not be set")
	}

	if err := protoMessage.Validate(); err != nil {
		return false, err
	}

	messageId := protoMessage.MessageId.Value
	ts := protoMessage.Ts

	// Clear out extracted metadata as a space optimization
	cloned := proto.Clone(protoMessage).(*chatpb.ChatMessage)
	cloned.MessageId = nil
	cloned.Ts = nil
	cloned.Cursor = nil

	marshalled, err := proto.Marshal(cloned)
	if err != nil {
		return false, err
	}

	canPersistMessage := true
	canPushMessage = !isSilentMessage

	existingChatRecord, err := data.GetChatById(ctx, chatId)
	switch err {
	case nil:
		canPersistMessage = !existingChatRecord.IsUnsubscribed
		canPushMessage = canPushMessage && canPersistMessage && !existingChatRecord.IsMuted
	case chat.ErrChatNotFound:
		chatRecord := &chat.Chat{
			ChatId:     chatId,
			ChatType:   chatType,
			ChatTitle:  chatTitle,
			IsVerified: isVerifiedChat,

			CodeUser: receiver.PublicKey().ToBase58(),

			ReadPointer:    nil,
			IsMuted:        false,
			IsUnsubscribed: false,

			CreatedAt: time.Now(),
		}

		err = data.PutChat(ctx, chatRecord)
		if err != nil && err != chat.ErrChatAlreadyExists {
			return false, err
		}
	default:
		return false, err
	}

	if canPersistMessage {
		messageRecord := &chat.Message{
			ChatId: chatId,

			MessageId: base58.Encode(messageId),
			Data:      marshalled,

			IsSilent:      isSilentMessage,
			ContentLength: uint8(len(protoMessage.Content)),

			Timestamp: ts.AsTime(),
		}

		err = data.PutChatMessage(ctx, messageRecord)
		if err != nil {
			return false, err
		}
	}

	if canPushMessage {
		err = data.AddToBadgeCount(ctx, receiver.PublicKey().ToBase58(), 1)
		if err != nil {
			return false, err
		}
	}

	return canPushMessage, nil
}
