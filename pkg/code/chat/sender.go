package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	chatv2pb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	chat_v2 "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	chatserver "github.com/code-payments/code-server/pkg/code/server/grpc/chat/v2"
)

// SendNotificationChatMessageV1 sends a chat message to a receiving owner account.
//
// Note: This function is not responsible for push notifications. This method
// might be called within the context of a DB transaction, which might have
// unrelated failures. A hint whether a push should be sent is provided.
func SendNotificationChatMessageV1(
	ctx context.Context,
	data code_data.Provider,
	chatTitle string,
	chatType chat_v1.ChatType,
	isVerifiedChat bool,
	receiver *common.Account,
	protoMessage *chatpb.ChatMessage,
	isSilentMessage bool,
) (canPushMessage bool, err error) {
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), isVerifiedChat)

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

	existingChatRecord, err := data.GetChatByIdV1(ctx, chatId)
	switch err {
	case nil:
		canPersistMessage = !existingChatRecord.IsUnsubscribed
		canPushMessage = canPushMessage && canPersistMessage && !existingChatRecord.IsMuted
	case chat_v1.ErrChatNotFound:
		chatRecord := &chat_v1.Chat{
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

		err = data.PutChatV1(ctx, chatRecord)
		if err != nil && !errors.Is(err, chat_v1.ErrChatAlreadyExists) {
			return false, err
		}
	default:
		return false, err
	}

	if canPersistMessage {
		messageRecord := &chat_v1.Message{
			ChatId: chatId,

			MessageId: base58.Encode(messageId),
			Data:      marshalled,

			IsSilent:      isSilentMessage,
			ContentLength: uint8(len(protoMessage.Content)),

			Timestamp: ts.AsTime(),
		}

		err = data.PutChatMessageV1(ctx, messageRecord)
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

func SendNotificationChatMessageV2(
	ctx context.Context,
	data code_data.Provider,
	notifier chatserver.Notifier,
	chatTitle string,
	isVerifiedChat bool,
	receiver *common.Account,
	protoMessage *chatv2pb.ChatMessage,
	intentId string,
	isSilentMessage bool,
) (canPushMessage bool, err error) {
	log := logrus.StandardLogger().WithField("type", "sendNotificationChatMessageV2")

	chatId := chat_v2.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), isVerifiedChat)

	if protoMessage.Cursor != nil {
		// Let the utilities and GetMessages RPC handle cursors
		return false, errors.New("cursor must not be set")
	}

	if err := protoMessage.Validate(); err != nil {
		return false, err
	}

	messageId, err := chat_v2.GetMessageIdFromProto(protoMessage.MessageId)
	if err != nil {
		return false, fmt.Errorf("invalid message id: %w", err)
	}

	// Clear out extracted metadata as a space optimization
	cloned := proto.Clone(protoMessage).(*chatv2pb.ChatMessage)
	cloned.MessageId = nil
	cloned.Ts = nil
	cloned.Cursor = nil

	marshalled, err := proto.Marshal(cloned)
	if err != nil {
		return false, err
	}

	canPersistMessage := true
	canPushMessage = !isSilentMessage

	//
	// Step 1: Check to see if we need to create the chat.
	//
	_, err = data.GetChatByIdV2(ctx, chatId)
	if errors.Is(err, chat_v2.ErrChatNotFound) {
		chatRecord := &chat_v2.ChatRecord{
			ChatId:     chatId,
			ChatType:   chat_v2.ChatTypeNotification,
			ChatTitle:  &chatTitle,
			IsVerified: isVerifiedChat,

			CreatedAt: time.Now(),
		}

		// TODO: These should be run in a transaction, but so far
		// we're being called in a transaction. We should have some kind
		// of safety check here...
		err = data.PutChatV2(ctx, chatRecord)
		if err != nil && !errors.Is(err, chat_v2.ErrChatExists) {
			return false, fmt.Errorf("failed to initialize chat: %w", err)
		}

		memberId := chat_v2.GenerateMemberId()
		err = data.PutChatMemberV2(ctx, &chat_v2.MemberRecord{
			ChatId:     chatId,
			MemberId:   memberId,
			Platform:   chat_v2.PlatformCode,
			PlatformId: receiver.PublicKey().ToBase58(),
			JoinedAt:   time.Now(),
		})
		if err != nil {
			return false, fmt.Errorf("failed to initialize chat with member: %w", err)
		}

		log.WithFields(logrus.Fields{
			"chat_id":     chatId.String(),
			"member":      memberId.String(),
			"platform_id": receiver.PublicKey().ToBase58(),
		}).Info("Initialized chat for tip")

	} else if err != nil {
		return false, err
	}

	//
	// Step 2: Ensure that there is exactly 1 member in the chat.
	//
	members, err := data.GetAllChatMembersV2(ctx, chatId)
	if errors.Is(err, chat_v2.ErrMemberNotFound) { // TODO: This is a weird error...
		return false, nil
	} else if err != nil {
		return false, err
	}
	if len(members) > 1 {
		// TODO: This _could_ get weird if client or someone else decides to join as another member.
		return false, errors.New("notification chat should have at most 1 member")
	}

	canPersistMessage = !members[0].IsUnsubscribed
	canPushMessage = canPushMessage && canPersistMessage && !members[0].IsMuted

	if canPersistMessage {
		refType := chat_v2.ReferenceTypeIntent
		messageRecord := &chat_v2.MessageRecord{
			ChatId:    chatId,
			MessageId: messageId,

			Data:     marshalled,
			IsSilent: isSilentMessage,

			ReferenceType: &refType,
			Reference:     &intentId,
		}

		// TODO: Once we have a better idea on the data modeling around chatv2,
		//       we may wish to have the server manage the creation of messages
		//       (and chats?) as well. That would also put the
		err = data.PutChatMessageV2(ctx, messageRecord)
		if err != nil {
			return false, err
		}

		notifier.NotifyMessage(ctx, chatId, protoMessage)

		log.WithField("chat_id", chatId.String()).Info("Put and notified")
	}

	// TODO: Once we move more things over to chatv2, we will need to increment
	//       badge count here. We don't currently, as it would result in a double
	//       push.
	return canPushMessage, nil
}
