package chat_v2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/localization"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

const (
	maxGetMessagesPageSize = 100
)

// todo: Ensure all relevant logging fields are set
type server struct {
	log *logrus.Entry

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
		log: logrus.StandardLogger().WithField("type", "chat/v2/server"),

		data: data,
		auth: auth,

		streams: make(map[string]*chatEventStream),

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

	return nil, status.Error(codes.Unimplemented, "")
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

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	memberId, err := chat.GetMemberIdFromProto(req.MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	}

	var limit uint64
	if req.PageSize > 0 {
		limit = uint64(req.PageSize)
	} else {
		limit = maxGetMessagesPageSize
	}
	if limit > maxGetMessagesPageSize {
		limit = maxGetMessagesPageSize
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

	messageRecords, err := s.data.GetAllChatMessagesV2(
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

	var userLocale *language.Tag // Loaded lazily when required
	var protoChatMessages []*chatpb.ChatMessage
	for _, messageRecord := range messageRecords {
		log := log.WithField("message_id", messageRecord.MessageId.String())

		var protoChatMessage chatpb.ChatMessage
		err = proto.Unmarshal(messageRecord.Data, &protoChatMessage)
		if err != nil {
			log.WithError(err).Warn("failure unmarshalling proto chat message")
			return nil, status.Error(codes.Internal, "")
		}

		ts, err := messageRecord.GetTimestamp()
		if err != nil {
			log.WithError(err).Warn("failure getting message timestamp")
			return nil, status.Error(codes.Internal, "")
		}

		for _, content := range protoChatMessage.Content {
			switch typed := content.Type.(type) {
			case *chatpb.Content_Localized:
				if userLocale == nil {
					loadedUserLocale, err := s.data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
					if err != nil {
						log.WithError(err).Warn("failure getting user locale")
						return nil, status.Error(codes.Internal, "")
					}
					userLocale = &loadedUserLocale
				}

				typed.Localized.KeyOrText = localization.LocalizeWithFallback(
					*userLocale,
					localization.GetLocalizationKeyForUserAgent(ctx, typed.Localized.KeyOrText),
					typed.Localized.KeyOrText,
				)
			}
		}

		protoChatMessage.MessageId = &chatpb.ChatMessageId{Value: messageRecord.MessageId[:]}
		if messageRecord.Sender != nil {
			protoChatMessage.SenderId = &chatpb.ChatMemberId{Value: messageRecord.Sender[:]}
		}
		protoChatMessage.Ts = timestamppb.New(ts)
		protoChatMessage.Cursor = &chatpb.Cursor{Value: messageRecord.MessageId[:]}
	}

	return &chatpb.GetMessagesResponse{
		Result:   chatpb.GetMessagesResponse_OK,
		Messages: protoChatMessages,
	}, nil
}

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

	owner, err := common.NewAccountFromProto(req.GetOpenStream().Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	chatId, err := chat.GetChatIdFromProto(req.GetOpenStream().ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	memberId, err := chat.GetMemberIdFromProto(req.GetOpenStream().MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.GetOpenStream().Signature
	req.GetOpenStream().Signature = nil
	if err = s.auth.Authenticate(streamer.Context(), owner, req.GetOpenStream(), signature); err != nil {
		return err
	}

	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return status.Error(codes.Unimplemented, "todo: missing result code")
	default:
		log.WithError(err).Warn("failure getting chat record")
		return status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return status.Error(codes.Unimplemented, "todo: missing result code")
	}

	streamKey := fmt.Sprintf("%s:%s", chatId.String(), memberId.String())

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

	defer func() {
		s.streamsMu.Lock()

		log.Tracef("closing streamer (stream=%s)", streamRef)

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamChatEvents() call is handling it.
		liveStream, exists := s.streams[streamKey]
		if exists && liveStream == stream {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()
	}()

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

	return nil, status.Error(codes.Unimplemented, "")
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

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	memberId, err := chat.GetMemberIdFromProto(req.Pointer.MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	pointerType := chat.GetPointerTypeFromProto(req.Pointer.Kind)
	log = log.WithField("pointer_type", pointerType.String())
	switch pointerType {
	case chat.PointerTypeDelivered, chat.PointerTypeRead:
	default:
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	}

	pointerValue, err := chat.GetMessageIdFromProto(req.Pointer.Value)
	if err != nil {
		log.WithError(err).Warn("invalid pointer value")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("pointer_value", pointerValue.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	}

	_, err = s.data.GetChatMessageByIdV2(ctx, chatId, pointerValue)
	switch err {
	case nil:
	case chat.ErrMessageNotFound:
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_MESSAGE_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat message record")
		return nil, status.Error(codes.Internal, "")
	}

	// Note: Guarantees that pointer will never be advanced to some point in the past
	err = s.data.AdvanceChatPointerV2(ctx, chatId, memberId, pointerType, pointerValue)
	if err != nil {
		log.WithError(err).Warn("failure advancing chat pointer")
		return nil, status.Error(codes.Internal, "")
	}

	event := &chatpb.ChatStreamEvent{
		Type: &chatpb.ChatStreamEvent_Pointer{
			Pointer: req.Pointer,
		},
	}
	if err := s.asyncNotifyAll(chatId, memberId, event); err != nil {
		log.WithError(err).Warn("failure notifying chat event")
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

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	memberId, err := chat.GetMemberIdFromProto(req.MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	// todo: Use chat record to determine if muting is allowed
	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return &chatpb.SetMuteStateResponse{
			Result: chatpb.SetMuteStateResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	isChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !isChatMember {
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	}

	err = s.data.SetChatMuteStateV2(ctx, chatId, memberId, req.IsMuted)
	if err != nil {
		log.WithError(err).Warn("failure setting mute state")
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

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	memberId, err := chat.GetMemberIdFromProto(req.MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	// todo: Use chat record to determine if muting is allowed
	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return &chatpb.SetSubscriptionStateResponse{
			Result: chatpb.SetSubscriptionStateResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return nil, status.Error(codes.Unimplemented, "todo: missing result code")
	}

	err = s.data.SetChatSubscriptionStateV2(ctx, chatId, memberId, req.IsSubscribed)
	if err != nil {
		log.WithError(err).Warn("failure setting mute state")
		return nil, status.Error(codes.Internal, "")
	}

	return &chatpb.SetSubscriptionStateResponse{
		Result: chatpb.SetSubscriptionStateResponse_OK,
	}, nil
}

func (s *server) ownsChatMember(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, owner *common.Account) (bool, error) {
	memberRecord, err := s.data.GetChatMemberByIdV2(ctx, chatId, memberId)
	switch err {
	case nil:
	case chat.ErrMemberNotFound:
		return false, nil
	default:
		return false, errors.Wrap(err, "error getting member record")
	}

	switch memberRecord.Platform {
	case chat.PlatformCode:
		return memberRecord.PlatformId == owner.PublicKey().ToBase58(), nil
	case chat.PlatformTwitter:
		// todo: This logic should live elsewhere in somewhere more common

		ownerTipAccount, err := owner.ToTimelockVault(timelock_token.DataVersion1, common.KinMintAccount)
		if err != nil {
			return false, errors.Wrap(err, "error deriving twitter tip address")
		}

		twitterRecord, err := s.data.GetTwitterUserByUsername(ctx, memberRecord.PlatformId)
		switch err {
		case nil:
		case twitter.ErrUserNotFound:
			return false, nil
		default:
			return false, errors.Wrap(err, "error getting twitter user")
		}

		return twitterRecord.TipAddress == ownerTipAccount.PublicKey().ToBase58(), nil
	default:
		return false, nil
	}
}
