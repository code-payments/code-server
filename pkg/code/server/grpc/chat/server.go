package chat

import (
	"context"
	"math"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/localization"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

const (
	maxPageSize = 100
)

type server struct {
	log  *logrus.Entry
	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	chatpb.UnimplementedChatServer
}

func NewChatServer(data code_data.Provider, auth *auth_util.RPCSignatureVerifier) chatpb.ChatServer {
	return &server{
		log:  logrus.StandardLogger().WithField("type", "chat/server"),
		data: data,
		auth: auth,
	}
}

func (s *server) GetChats(ctx context.Context, req *chatpb.GetChatsRequest) (*chatpb.GetChatsResponse, error) {
	log := s.log.WithField("method", "GetChats")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	var limit uint64
	if req.PageSize > 0 {
		limit = uint64(req.PageSize)
	} else {
		limit = maxPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	var direction query.Ordering
	if req.Direction == chatpb.GetChatsRequest_ASC {
		direction = query.Ascending
	} else {
		direction = query.Descending
	}

	var cursor query.Cursor
	if req.Cursor != nil {
		cursor = req.Cursor.Value
	} else {
		cursor = query.ToCursor(0)
		if direction == query.Descending {
			cursor = query.ToCursor(math.MaxInt64 - 1)
		}
	}

	chatRecords, err := s.data.GetAllChatsForUser(
		ctx,
		owner.PublicKey().ToBase58(),
		query.WithCursor(cursor),
		query.WithDirection(direction),
		query.WithLimit(limit),
	)
	if err == chat.ErrChatNotFound {
		return &chatpb.GetChatsResponse{
			Result: chatpb.GetChatsResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat records")
		return nil, status.Error(codes.Internal, "")
	}

	var protoMetadatas []*chatpb.ChatMetadata
	for _, chatRecord := range chatRecords {
		log := log.WithField("chat_id", chatRecord.ChatId.String())

		protoMetadata := &chatpb.ChatMetadata{
			ChatId:         chatRecord.ChatId.ToProto(),
			IsVerified:     chatRecord.IsVerified,
			IsMuted:        chatRecord.IsMuted,
			IsSubscribed:   !chatRecord.IsUnsubscribed,
			CanMute:        true,
			CanUnsubscribe: true,
			Cursor: &chatpb.Cursor{
				Value: query.ToCursor(chatRecord.Id),
			},
		}

		// Unread count calculations can be skipped for unsubscribed chats. They
		// don't appear in chat history.
		skipUnreadCountQuery := chatRecord.IsUnsubscribed

		switch chatRecord.ChatType {
		case chat.ChatTypeInternal:
			chatProperties, ok := chat_util.InternalChatProperties[chatRecord.ChatTitle]
			if !ok {
				log.Warnf("%s chat doesn't have properties defined", chatRecord.ChatTitle)
				continue
			}

			// All messages for cash transactions and payments are silent, and this
			// history may be long, so we can skip the unread count calculation as
			// an optimization.
			switch chatRecord.ChatTitle {
			case chat_util.CashTransactionsName, chat_util.PaymentsName:
				skipUnreadCountQuery = true
			}

			protoMetadata.Title = &chatpb.ChatMetadata_Localized{
				Localized: &chatpb.LocalizedContent{
					KeyOrText: localization.GetLocalizationKeyForUserAgent(ctx, chatProperties.TitleLocalizationKey),
				},
			}
			protoMetadata.CanMute = chatProperties.CanMute
			protoMetadata.CanUnsubscribe = chatProperties.CanUnsubscribe
		case chat.ChatTypeExternalApp:
			protoMetadata.Title = &chatpb.ChatMetadata_Domain{
				Domain: &commonpb.Domain{
					Value: chatRecord.ChatTitle,
				},
			}

			// All messages for unverified merchant chats are silent, and this history
			// may be long, so we can skip the unread count calculation as an optimization.
			skipUnreadCountQuery = skipUnreadCountQuery || !chatRecord.IsVerified
		default:
			log.WithField("chat_type", chatRecord.ChatType).Warn("unhandled chat type")
			return nil, status.Error(codes.Internal, "")
		}

		if chatRecord.ReadPointer != nil {
			pointerValue, err := base58.Decode(*chatRecord.ReadPointer)
			if err != nil {
				log.WithError(err).Warn("failure decoding read pointer value")
				return nil, status.Error(codes.Internal, "")
			}

			protoMetadata.ReadPointer = &chatpb.Pointer{
				Kind: chatpb.Pointer_READ,
				Value: &chatpb.ChatMessageId{
					Value: pointerValue,
				},
			}
		}

		if !skipUnreadCountQuery && !chatRecord.IsMuted && !chatRecord.IsUnsubscribed {
			// todo: will need batching when users have a large number of chats
			unreadCount, err := s.data.GetChatUnreadCount(ctx, chatRecord.ChatId)
			if err != nil {
				log.WithError(err).Warn("failure getting unread count")
			}
			protoMetadata.NumUnread = unreadCount
		}

		protoMetadatas = append(protoMetadatas, protoMetadata)

		if len(protoMetadatas) >= maxPageSize {
			break
		}
	}

	return &chatpb.GetChatsResponse{
		Result: chatpb.GetChatsResponse_OK,
		Chats:  protoMetadatas,
	}, nil
}

func (s *server) GetMessages(ctx context.Context, req *chatpb.GetMessagesRequest) (*chatpb.GetMessagesResponse, error) {
	log := s.log.WithField("method", "GetMessages")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	chatId := chat.ChatIdFromProto(req.ChatId)
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	chatRecord, err := s.data.GetChatById(ctx, chatId)
	if err == chat.ErrChatNotFound {
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	if chatRecord.CodeUser != owner.PublicKey().ToBase58() {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	var limit uint64
	if req.PageSize > 0 {
		limit = uint64(req.PageSize)
	} else {
		limit = maxPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	var direction query.Ordering
	if req.Direction == chatpb.GetMessagesRequest_ASC {
		direction = query.Ascending
	} else {
		direction = query.Descending
	}

	var cursor query.Cursor
	if req.Cursor != nil {
		cursor = req.Cursor.Value
	}

	messageRecords, err := s.data.GetAllChatMessages(
		ctx,
		chatId,
		query.WithCursor(cursor),
		query.WithDirection(direction),
		query.WithLimit(limit),
	)
	if err == chat.ErrMessageNotFound {
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat message records")
		return nil, status.Error(codes.Internal, "")
	}

	var protoChatMessages []*chatpb.ChatMessage
	for _, messageRecord := range messageRecords {
		var protoChatMessage chatpb.ChatMessage
		err = proto.Unmarshal(messageRecord.Data, &protoChatMessage)
		if err != nil {
			log.WithError(err).Warn("failure unmarshalling proto chat message")
			return nil, status.Error(codes.Internal, "")
		}

		for _, content := range protoChatMessage.Content {
			switch typed := content.Type.(type) {
			case *chatpb.Content_Localized:
				typed.Localized.KeyOrText = localization.GetLocalizationKeyForUserAgent(ctx, typed.Localized.KeyOrText)
			}
		}

		messageIdBytes, err := base58.Decode(messageRecord.MessageId)
		if err != nil {
			log.WithError(err).Warn("failure decoding chat message id")
			return nil, status.Error(codes.Internal, "")
		}

		protoChatMessage.MessageId = &chatpb.ChatMessageId{
			Value: messageIdBytes,
		}
		protoChatMessage.Ts = timestamppb.New(messageRecord.Timestamp)
		protoChatMessage.Cursor = &chatpb.Cursor{
			// Non-standard cursor because we index on time and don't have append
			// semantics to the message log
			Value: messageIdBytes,
		}

		protoChatMessages = append(protoChatMessages, &protoChatMessage)

		if len(protoChatMessages) >= maxPageSize {
			break
		}
	}

	return &chatpb.GetMessagesResponse{
		Result:   chatpb.GetMessagesResponse_OK,
		Messages: protoChatMessages,
	}, nil
}

func (s *server) AdvancePointer(ctx context.Context, req *chatpb.AdvancePointerRequest) (*chatpb.AdvancePointerResponse, error) {
	log := s.log.WithField("method", "AdvancePointer")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	chatId := chat.ChatIdFromProto(req.ChatId)
	log = log.WithField("chat_id", chatId.String())

	messageId := base58.Encode(req.Pointer.Value.Value)
	log = log.WithFields(logrus.Fields{
		"message_id":   messageId,
		"pointer_type": req.Pointer.Kind,
	})

	if req.Pointer.Kind != chatpb.Pointer_READ {
		return nil, status.Error(codes.InvalidArgument, "Pointer.Kind must be READ")
	}

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	chatRecord, err := s.data.GetChatById(ctx, chatId)
	if err == chat.ErrChatNotFound {
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_CHAT_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	if chatRecord.CodeUser != owner.PublicKey().ToBase58() {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	newPointerRecord, err := s.data.GetChatMessage(ctx, chatId, messageId)
	if err == chat.ErrMessageNotFound {
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_MESSAGE_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat message record for new pointer value")
		return nil, status.Error(codes.Internal, "")
	}

	if chatRecord.ReadPointer != nil {
		oldPointerRecord, err := s.data.GetChatMessage(ctx, chatId, *chatRecord.ReadPointer)
		if err != nil {
			log.WithError(err).Warn("failure getting chat message record for old pointer value")
			return nil, status.Error(codes.Internal, "")
		}

		if oldPointerRecord.MessageId == newPointerRecord.MessageId || oldPointerRecord.Timestamp.After(newPointerRecord.Timestamp) {
			return &chatpb.AdvancePointerResponse{
				Result: chatpb.AdvancePointerResponse_OK,
			}, nil
		}
	}

	err = s.data.AdvanceChatPointer(ctx, chatId, messageId)
	if err != nil {
		log.WithError(err).Warn("failure advancing pointer")
		return nil, status.Error(codes.Internal, "")
	}
	return &chatpb.AdvancePointerResponse{
		Result: chatpb.AdvancePointerResponse_OK,
	}, nil
}

func (s *server) SetMuteState(ctx context.Context, req *chatpb.SetMuteStateRequest) (*chatpb.SetMuteStateResponse, error) {
	log := s.log.WithField("method", "SetMuteState")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	chatId := chat.ChatIdFromProto(req.ChatId)
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	chatRecord, err := s.data.GetChatById(ctx, chatId)
	if err == chat.ErrChatNotFound {
		return &chatpb.SetMuteStateResponse{
			Result: chatpb.SetMuteStateResponse_CHAT_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	if chatRecord.CodeUser != owner.PublicKey().ToBase58() {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if chatRecord.IsMuted == req.IsMuted {
		return &chatpb.SetMuteStateResponse{
			Result: chatpb.SetMuteStateResponse_OK,
		}, nil
	}

	chatProperties, ok := chat_util.InternalChatProperties[chatRecord.ChatTitle]
	if ok && req.IsMuted && !chatProperties.CanMute {
		return &chatpb.SetMuteStateResponse{
			Result: chatpb.SetMuteStateResponse_CANT_MUTE,
		}, nil
	}

	err = s.data.SetChatMuteState(ctx, chatId, req.IsMuted)
	if err != nil {
		log.WithError(err).Warn("failure setting mute status")
		return nil, status.Error(codes.Internal, "")
	}

	return &chatpb.SetMuteStateResponse{
		Result: chatpb.SetMuteStateResponse_OK,
	}, nil
}

func (s *server) SetSubscriptionState(ctx context.Context, req *chatpb.SetSubscriptionStateRequest) (*chatpb.SetSubscriptionStateResponse, error) {
	log := s.log.WithField("method", "SetSubscriptionState")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	chatId := chat.ChatIdFromProto(req.ChatId)
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	chatRecord, err := s.data.GetChatById(ctx, chatId)
	if err == chat.ErrChatNotFound {
		return &chatpb.SetSubscriptionStateResponse{
			Result: chatpb.SetSubscriptionStateResponse_CHAT_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	if chatRecord.CodeUser != owner.PublicKey().ToBase58() {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if chatRecord.IsUnsubscribed != req.IsSubscribed {
		return &chatpb.SetSubscriptionStateResponse{
			Result: chatpb.SetSubscriptionStateResponse_OK,
		}, nil
	}

	chatProperties, ok := chat_util.InternalChatProperties[chatRecord.ChatTitle]
	if ok && !req.IsSubscribed && !chatProperties.CanUnsubscribe {
		return &chatpb.SetSubscriptionStateResponse{
			Result: chatpb.SetSubscriptionStateResponse_CANT_UNSUBSCRIBE,
		}, nil
	}

	err = s.data.SetChatSubscriptionState(ctx, chatId, req.IsSubscribed)
	if err != nil {
		log.WithError(err).Warn("failure setting subcription status")
		return nil, status.Error(codes.Internal, "")
	}

	return &chatpb.SetSubscriptionStateResponse{
		Result: chatpb.SetSubscriptionStateResponse_OK,
	}, nil
}
