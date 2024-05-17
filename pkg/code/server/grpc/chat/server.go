package chat

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
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
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

const (
	maxPageSize = 100
)

var (
	mockTwoWayChat = chat.GetChatId("user1", "user2", true).ToProto()
)

// todo: Resolve duplication of streaming logic with messaging service. The latest and greatest will live here.
type server struct {
	log  *logrus.Entry
	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	streamsMu sync.RWMutex
	streams   map[string]*chatEventStream

	chatLocks      *sync_util.StripedLock
	chatEventChans *sync_util.StripedChannel

	chatpb.UnimplementedChatServer
}

func NewChatServer(data code_data.Provider, auth *auth_util.RPCSignatureVerifier) chatpb.ChatServer {
	s := &server{
		log:            logrus.StandardLogger().WithField("type", "chat/server"),
		data:           data,
		auth:           auth,
		streams:        make(map[string]*chatEventStream),
		chatLocks:      sync_util.NewStripedLock(64),             // todo: configurable parameters
		chatEventChans: sync_util.NewStripedChannel(64, 100_000), // todo: configurable parameters
	}

	for i, channel := range s.chatEventChans.GetChannels() {
		go s.asyncChatEventStreamNotifier(i, channel)
	}

	return s
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

	locale, err := s.data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
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
				Localized: &chatpb.ServerLocalizedContent{
					KeyOrText: localization.LocalizeWithFallback(
						locale,
						localization.GetLocalizationKeyForUserAgent(ctx, chatProperties.TitleLocalizationKey),
						chatProperties.TitleLocalizationKey,
					),
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

	locale, err := s.data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
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
			case *chatpb.Content_ServerLocalized:
				typed.ServerLocalized.KeyOrText = localization.LocalizeWithFallback(
					locale,
					localization.GetLocalizationKeyForUserAgent(ctx, typed.ServerLocalized.KeyOrText),
					typed.ServerLocalized.KeyOrText,
				)
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

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	chatId := chat.ChatIdFromProto(req.ChatId)
	messageId := base58.Encode(req.Pointer.Value.Value)
	log = log.WithFields(logrus.Fields{
		"chat_id":      chatId.String(),
		"message_id":   messageId,
		"pointer_type": req.Pointer.Kind,
	})

	// todo: Temporary code to simluate real-time
	if req.Pointer.User != nil {
		return nil, status.Error(codes.InvalidArgument, "pointer.user cannot be set by clients")
	}
	if bytes.Equal(mockTwoWayChat.Value, req.ChatId.Value) {
		req.Pointer.User = &chatpb.ChatMemberId{Value: req.Owner.Value}

		event := &chatpb.ChatStreamEvent{
			Pointers: []*chatpb.Pointer{req.Pointer},
		}

		if err := s.asyncNotifyAll(chatId, owner, event); err != nil {
			log.WithError(err).Warn("failure notifying chat event")
		}

		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_OK,
		}, nil
	}

	if req.Pointer.Kind != chatpb.Pointer_READ {
		return nil, status.Error(codes.InvalidArgument, "Pointer.Kind must be READ")
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

//
// Experimental PoC two-way chat APIs below
//

func (s *server) StreamChatEvents(streamer chatpb.Chat_StreamChatEventsServer) error {
	ctx := streamer.Context()

	log := s.log.WithField("method", "StreamChatEvents")
	log = client.InjectLoggingMetadata(ctx, log)

	req, err := boundedStreamChatEventsRecv(ctx, streamer, 250*time.Millisecond)
	if err != nil {
		return err
	}

	if req.GetOpenStream() == nil {
		return status.Error(codes.InvalidArgument, "open_stream is nil")
	}

	if req.GetOpenStream().Signature == nil {
		return status.Error(codes.InvalidArgument, "signature is nil")
	}

	if !bytes.Equal(req.GetOpenStream().ChatId.Value, mockTwoWayChat.Value) {
		return status.Error(codes.Unimplemented, "")
	}
	chatId := chat.ChatIdFromProto(req.GetOpenStream().ChatId)
	log = log.WithField("chat_id", chatId.String())

	owner, err := common.NewAccountFromProto(req.GetOpenStream().Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	signature := req.GetOpenStream().Signature
	req.GetOpenStream().Signature = nil
	if err = s.auth.Authenticate(streamer.Context(), owner, req.GetOpenStream(), signature); err != nil {
		return err
	}

	streamKey := fmt.Sprintf("%s:%s", chatId.String(), owner.PublicKey().ToBase58())

	s.streamsMu.Lock()

	stream, exists := s.streams[streamKey]
	if exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this server (stream=%p) ; aborting", stream)
		return status.Error(codes.Aborted, "stream already exists")
	}

	stream = newChatEventStream(streamBufferSize)

	// The race detector complains when reading the stream pointer ref outside of the lock.
	streamRef := fmt.Sprintf("%p", stream)
	log.Tracef("setting up new stream (stream=%s)", streamRef)
	s.streams[streamKey] = stream

	s.streamsMu.Unlock()

	sendPingCh := time.After(0)
	streamHealthCh := monitorChatEventStreamHealth(ctx, log, streamRef, streamer)

	for {
		select {
		case event, ok := <-stream.streamCh:
			if !ok {
				log.Tracef("stream closed ; ending stream (stream=%s)", streamRef)
				return status.Error(codes.Aborted, "stream closed")
			}

			err := streamer.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Events{
					Events: &chatpb.ChatStreamEventBatch{
						Events: []*chatpb.ChatStreamEvent{event},
					},
				},
			})
			if err != nil {
				log.WithError(err).Info("failed to forward chat message")
				return err
			}
		case <-sendPingCh:
			log.Tracef("sending ping to client (stream=%s)", streamRef)

			sendPingCh = time.After(streamPingDelay)

			err := streamer.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Ping{
					Ping: &commonpb.ServerPing{
						Timestamp: timestamppb.Now(),
						PingDelay: durationpb.New(streamPingDelay),
					},
				},
			})
			if err != nil {
				log.Tracef("stream is unhealthy ; aborting (stream=%s)", streamRef)
				return status.Error(codes.Aborted, "terminating unhealthy stream")
			}
		case <-streamHealthCh:
			log.Tracef("stream is unhealthy ; aborting (stream=%s)", streamRef)
			return status.Error(codes.Aborted, "terminating unhealthy stream")
		case <-ctx.Done():
			log.Tracef("stream context cancelled ; ending stream (stream=%s)", streamRef)
			return status.Error(codes.Canceled, "")
		}
	}
}

func (s *server) SendMessage(ctx context.Context, req *chatpb.SendMessageRequest) (*chatpb.SendMessageResponse, error) {
	log := s.log.WithField("method", "SendMessage")
	log = client.InjectLoggingMetadata(ctx, log)

	if !bytes.Equal(req.ChatId.Value, mockTwoWayChat.Value) {
		return nil, status.Error(codes.Unimplemented, "")
	}
	chatId := chat.ChatIdFromProto(req.ChatId)
	log = log.WithField("chat_id", chatId.String())

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err = s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	switch req.Content[0].Type.(type) {
	case *chatpb.Content_UserText:
	default:
		return nil, status.Error(codes.InvalidArgument, "content[0] must be UserText")
	}

	chatLock := s.chatLocks.Get(chatId[:])
	chatLock.Lock()
	defer chatLock.Unlock()

	// todo: Revisit message IDs
	messageId, err := common.NewRandomAccount()
	if err != nil {
		log.WithError(err).Warn("failure generating random message id")
		return nil, status.Error(codes.Internal, "")
	}

	chatMessage := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{Value: messageId.ToProto().Value},
		Ts:        timestamppb.Now(),
		Content:   req.Content,
		Sender:    &chatpb.ChatMemberId{Value: req.Owner.Value},
		Cursor:    nil, // todo: Don't have cursor until we save it to the DB
	}

	// todo: Save the message to the DB

	event := &chatpb.ChatStreamEvent{
		Messages: []*chatpb.ChatMessage{chatMessage},
	}

	if err := s.asyncNotifyAll(chatId, owner, event); err != nil {
		log.WithError(err).Warn("failure notifying chat event")
	}

	return &chatpb.SendMessageResponse{
		Result:  chatpb.SendMessageResponse_OK,
		Message: chatMessage,
	}, nil
}
