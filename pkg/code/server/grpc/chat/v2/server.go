package chat_v2

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/pointer"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/localization"
	push_util "github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/push"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

// todo: resolve some common code for sending chat messages across RPCs

const (
	maxGetChatsPageSize    = 100
	maxGetMessagesPageSize = 100
	flushMessageCount      = 100
)

type Server struct {
	log *logrus.Entry

	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier
	push push.Provider

	streamsMu sync.RWMutex
	streams   map[string]*chatEventStream

	chatLocks      *sync_util.StripedLock
	chatEventChans *sync_util.StripedChannel

	chatpb.UnimplementedChatServer
}

func NewChatServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
	push push.Provider,
) *Server {
	s := &Server{
		log: logrus.StandardLogger().WithField("type", "chat/v2/Server"),

		data: data,
		auth: auth,
		push: push,

		streams: make(map[string]*chatEventStream),

		chatLocks:      sync_util.NewStripedLock(64),             // todo: configurable parameters
		chatEventChans: sync_util.NewStripedChannel(64, 100_000), // todo: configurable parameters
	}

	for i, channel := range s.chatEventChans.GetChannels() {
		go s.asyncChatEventStreamNotifier(i, channel)
	}

	return s
}

func (s *Server) GetChats(ctx context.Context, req *chatpb.GetChatsRequest) (*chatpb.GetChatsResponse, error) {
	// todo: This will require a lot of optimizations since we iterate and make several DB calls for each chat membership
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
		limit = maxGetChatsPageSize
	}
	if limit > maxGetChatsPageSize {
		limit = maxGetChatsPageSize
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
	}

	memberID, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("Failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}

	chats, err := s.data.GetAllChatsForUserV2(
		ctx,
		memberID,
		query.WithCursor(cursor),
		query.WithDirection(direction),
		query.WithLimit(limit),
	)

	log.WithField("chats", len(chats)).Info("Retrieved chatlist for user")
	metadata := make([]*chatpb.Metadata, 0, len(chats))
	for _, id := range chats {
		md, err := s.getMetadata(ctx, memberID, id)
		if err != nil {
			return nil, nil
		}

		metadata = append(metadata, md)
	}

	return &chatpb.GetChatsResponse{
		Result: chatpb.GetChatsResponse_OK,
		Chats:  metadata,
	}, nil
}

func (s *Server) GetMessages(ctx context.Context, req *chatpb.GetMessagesRequest) (*chatpb.GetMessagesResponse, error) {
	log := s.log.WithField("method", "GetMessages")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	isChatMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failed to check if chat member")
		return nil, status.Error(codes.Internal, "")
	}
	if !isChatMember {
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_DENIED,
		}, nil
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

	protoChatMessages, err := s.getProtoChatMessages(
		ctx,
		chatId,
		owner,
		query.WithCursor(cursor),
		query.WithDirection(direction),
		query.WithLimit(limit),
	)
	if err != nil {
		log.WithError(err).Warn("failure getting chat messages")
		return nil, status.Error(codes.Internal, "")
	}

	return &chatpb.GetMessagesResponse{
		Result:   chatpb.GetMessagesResponse_OK,
		Messages: protoChatMessages,
	}, nil
}

func (s *Server) StreamChatEvents(streamer chatpb.Chat_StreamChatEventsServer) error {
	ctx := streamer.Context()

	log := s.log.WithField("method", "StreamChatEvents")
	log = client.InjectLoggingMetadata(ctx, log)

	req, err := boundedStreamChatEventsRecv(ctx, streamer, 250*time.Millisecond)
	if err != nil {
		return err
	}

	if req.GetOpenStream() == nil {
		return status.Error(codes.InvalidArgument, "StreamChatEventsRequest.Type must be OpenStreamRequest")
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

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.GetOpenStream().Signature
	req.GetOpenStream().Signature = nil
	if err := s.auth.Authenticate(streamer.Context(), owner, req.GetOpenStream(), signature); err != nil {
		return err
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return status.Error(codes.Internal, "")
	}
	if !isMember {
		return streamer.Send(&chatpb.StreamChatEventsResponse{
			Type: &chatpb.StreamChatEventsResponse_Error{
				Error: &chatpb.ChatStreamEventError{Code: chatpb.ChatStreamEventError_DENIED},
			},
		})
	}

	streamKey := fmt.Sprintf("%s:%s", chatId.String(), memberId.String())

	s.streamsMu.Lock()

	stream, exists := s.streams[streamKey]
	if exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this Server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this Server (stream=%p) ; aborting", stream)
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

	// todo: We should also "flush" pointers for each chat member
	go s.flushMessages(ctx, chatId, owner, stream)

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

func (s *Server) StartChat(ctx context.Context, req *chatpb.StartChatRequest) (*chatpb.StartChatResponse, error) {
	log := s.log.WithField("method", "StartChat")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive member id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId)

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	creator, err := s.data.GetTwitterUserByTipAddress(ctx, memberId.String())
	if errors.Is(err, twitter.ErrUserNotFound) {
		log.WithField("memberId", memberId).Info("User has no twitter account")
		return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_MISSING_IDENTITY}, nil
	} else if err != nil {
		log.WithError(err).Warn("failed to get twitter user")
		return nil, status.Error(codes.Internal, "")
	}

	switch typed := req.Parameters.(type) {
	case *chatpb.StartChatRequest_TwoWayChat:
		chatId := chat.GetTwoWayChatId(memberId, typed.TwoWayChat.OtherUser.Value)

		metadata, err := s.getMetadata(ctx, memberId, chatId)
		if err == nil {
			return &chatpb.StartChatResponse{
				Chat: metadata,
			}, nil

		} else if err != nil && !errors.Is(err, chat.ErrChatNotFound) {
			log.WithError(err).Warn("failed to get chat metadata")
			return nil, status.Error(codes.Internal, "")
		}

		if typed.TwoWayChat.IntentId == nil {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_INVALID_PARAMETER}, nil
		}

		intentId := base58.Encode(typed.TwoWayChat.IntentId.Value)
		log = log.WithField("intent", intentId)

		intentRecord, err := s.data.GetIntent(ctx, intentId)
		if errors.Is(err, intent.ErrIntentNotFound) {
			log.WithError(err).Info("Intent not found")
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_INVALID_PARAMETER,
				Chat:   nil,
			}, nil
		} else if err != nil {
			log.WithError(err).Warn("failure getting intent record")
			return nil, status.Error(codes.Internal, "")
		}

		switch intentRecord.State {
		case intent.StatePending:
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_PENDING,
			}, nil
		case intent.StateConfirmed:
		default:
			log.WithField("state", intentRecord.State).Info("PayToChat intent did not succeed")
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		if intentRecord.SendPrivatePaymentMetadata == nil {
			log.Info("intent missing private payment meta")
			//return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		if !intentRecord.SendPrivatePaymentMetadata.IsChat {
			log.Info("intent is not for chat")
			//return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		expectedChatId := base58.Encode(chatId[:])
		if intentRecord.SendPrivatePaymentMetadata.ChatId != expectedChatId {
			log.WithField("expected", expectedChatId).WithField("actual", intentRecord.SendPrivatePaymentMetadata.ChatId).Warn("chat id mismatch")
		}

		otherMessagingAddress := base58.Encode(typed.TwoWayChat.OtherUser.Value)

		otherTwitter, err := s.data.GetTwitterUserByTipAddress(ctx, otherMessagingAddress)
		if errors.Is(err, twitter.ErrUserNotFound) {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_USER_NOT_FOUND}, nil
		} else if err != nil {
			log.WithError(err).Warn("failure checking twitter user")
			return nil, status.Error(codes.Internal, "")
		}

		otherAccount, err := s.data.GetAccountInfoByTokenAddress(ctx, otherMessagingAddress)
		if errors.Is(err, account.ErrAccountInfoNotFound) {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_USER_NOT_FOUND}, nil
		} else if err != nil {
			log.WithError(err).Warn("failure checking account info")
			return nil, status.Error(codes.Internal, "")
		}

		// At this point, we assume the relationship is valid, and can proceed to recover or create
		// the chat record.
		creationTs := time.Now()
		chatRecord := &chat.MetadataRecord{
			ChatId:    chatId,
			ChatType:  chat.ChatTypeTwoWay,
			CreatedAt: creationTs,
			ChatTitle: nil,
		}

		memberRecords := []*chat.MemberRecord{
			{
				ChatId:   chatId,
				MemberId: memberId.String(),
				Owner:    owner.PublicKey().ToBase58(),

				Platform:   chat.PlatformTwitter,
				PlatformId: creator.Username,

				JoinedAt: creationTs,
			},
			{
				ChatId:   chatId,
				MemberId: otherMessagingAddress,
				Owner:    otherAccount.OwnerAccount,

				Platform:   chat.PlatformTwitter,
				PlatformId: otherTwitter.Username,

				JoinedAt: time.Now(),
			},
		}

		// Note: this should almost _always_ succeed in the happy path, since we check
		//       for existence earlier!
		//
		// The only time we have to rollback and query is on race of creation.
		err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
			existingChatRecord, err := s.data.GetChatMetadata(ctx, chatId)
			if err != nil && !errors.Is(err, chat.ErrChatNotFound) {
				return fmt.Errorf("failed to check existing chat: %w", err)
			}

			if existingChatRecord != nil {
				chatRecord = existingChatRecord
				memberRecords, err = s.data.GetChatMembersV2(ctx, chatId)
				if err != nil {
					return fmt.Errorf("failed to check existing chat members: %w", err)
				}

				return nil
			}

			if err = s.data.PutChatV2(ctx, chatRecord); err != nil {
				return fmt.Errorf("failed to save new chat: %w", err)
			}
			for _, m := range memberRecords {
				if err = s.data.PutChatMemberV2(ctx, m); err != nil {
					return fmt.Errorf("failed to add member to chat: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			log.WithError(err).Warn("failure creating chat")
			return nil, status.Error(codes.Internal, "")
		}

		md, err := s.populateMetadata(ctx, chatRecord, memberRecords, memberId)
		if err != nil {
			log.WithError(err).Warn("failure populating metadata")
			return nil, status.Error(codes.Internal, "")
		}

		return &chatpb.StartChatResponse{
			Result: chatpb.StartChatResponse_OK,
			Chat:   md,
		}, nil

	default:
		return nil, status.Error(codes.InvalidArgument, "StartChatRequest.Parameters is nil")
	}
}

func (s *Server) SendMessage(ctx context.Context, req *chatpb.SendMessageRequest) (*chatpb.SendMessageResponse, error) {
	log := s.log.WithField("method", "SendMessage")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId)

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	switch req.Content[0].Type.(type) {
	case *chatpb.Content_Text:
	default:
		return &chatpb.SendMessageResponse{
			Result: chatpb.SendMessageResponse_INVALID_CONTENT_TYPE,
		}, nil
	}

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	metadata, err := s.data.GetChatMetadata(ctx, chatId)
	if errors.Is(err, chat.ErrChatNotFound) {
		return &chatpb.SendMessageResponse{
			Result: chatpb.SendMessageResponse_DENIED,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failure checking member record")
		return nil, status.Error(codes.Internal, "")
	}
	if !isMember {
		return &chatpb.SendMessageResponse{
			Result: chatpb.SendMessageResponse_DENIED,
		}, nil
	}

	chatLock := s.chatLocks.Get(chatId[:])
	chatLock.Lock()
	defer chatLock.Unlock()

	chatMessage := newProtoChatMessage(memberId, req.Content...)

	err = s.persistChatMessage(ctx, chatId, chatMessage)
	if err != nil {
		log.WithError(err).Warn("failure persisting chat message")
		return nil, status.Error(codes.Internal, "")
	}

	s.onPersistChatMessage(log, chatId, chatMessage)
	s.sendPushNotifications(chatId, pointer.StringOrEmpty(metadata.ChatTitle), memberId, chatMessage)

	return &chatpb.SendMessageResponse{
		Result:  chatpb.SendMessageResponse_OK,
		Message: chatMessage,
	}, nil
}

func (s *Server) AdvancePointer(ctx context.Context, req *chatpb.AdvancePointerRequest) (*chatpb.AdvancePointerResponse, error) {
	log := s.log.WithField("method", "AdvancePointer")
	log = client.InjectLoggingMetadata(ctx, log)

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId)

	pointerType := chat.GetPointerTypeFromProto(req.Pointer.Type)
	log = log.WithField("pointer_type", pointerType.String())
	if pointerType <= chat.PointerTypeUnknown || pointerType > chat.PointerTypeRead {
		return nil, status.Error(codes.InvalidArgument, "invalid pointer type")
	}

	pointerValue, err := chat.GetMessageIdFromProto(req.Pointer.Value)
	if err != nil {
		log.WithError(err).Warn("invalid pointer value")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("pointer_value", pointerValue.String())

	// Force override whatever the user thought it should be.
	req.Pointer.MemberId = memberId.ToProto()

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failure checking member record")
		return nil, status.Error(codes.Internal, "")
	}
	if !isMember {
		return &chatpb.AdvancePointerResponse{Result: chatpb.AdvancePointerResponse_DENIED}, nil
	}

	_, err = s.data.GetChatMessageV2(ctx, chatId, pointerValue)
	if errors.Is(err, chat.ErrChatNotFound) {
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_MESSAGE_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat message record")
		return nil, status.Error(codes.Internal, "")
	}

	isAdvanced, err := s.data.AdvanceChatPointerV2(ctx, chatId, memberId, pointerType, pointerValue)
	if err != nil {
		log.WithError(err).Warn("failure advancing chat pointer")
		return nil, status.Error(codes.Internal, "")
	}

	if isAdvanced {
		event := &chatpb.ChatStreamEvent{
			Type: &chatpb.ChatStreamEvent_Pointer{
				Pointer: req.Pointer,
			},
		}
		if err := s.asyncNotifyAll(chatId, event); err != nil {
			log.WithError(err).Warn("failure notifying chat event")
		}
	}

	return &chatpb.AdvancePointerResponse{
		Result: chatpb.AdvancePointerResponse_OK,
	}, nil
}

func (s *Server) SetMuteState(ctx context.Context, req *chatpb.SetMuteStateRequest) (*chatpb.SetMuteStateResponse, error) {
	log := s.log.WithField("method", "SetMuteState")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failure checking member record")
		return nil, status.Error(codes.Internal, "")
	}
	if !isMember {
		return &chatpb.SetMuteStateResponse{Result: chatpb.SetMuteStateResponse_DENIED}, nil
	}

	// todo: Use chat record to determine if muting is allowed

	err = s.data.SetChatMuteStateV2(ctx, chatId, memberId, req.IsMuted)
	if err != nil {
		log.WithError(err).Warn("failure setting mute state")
		return nil, status.Error(codes.Internal, "")
	}

	return &chatpb.SetMuteStateResponse{
		Result: chatpb.SetMuteStateResponse_OK,
	}, nil
}

func (s *Server) NotifyIsTyping(ctx context.Context, req *chatpb.NotifyIsTypingRequest) (*chatpb.NotifyIsTypingResponse, error) {
	log := s.log.WithField("method", "NotifyIsTyping")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId)

	chatId, err := chat.GetChatIdFromProto(req.ChatId)
	if err != nil {
		log.WithError(err).Warn("invalid chat id")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("chat_id", chatId.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failure checking member record")
		return nil, status.Error(codes.Internal, "")
	}
	if !isMember {
		return &chatpb.NotifyIsTypingResponse{Result: chatpb.NotifyIsTypingResponse_DENIED}, nil
	}

	event := &chatpb.ChatStreamEvent{
		Type: &chatpb.ChatStreamEvent_IsTyping{
			IsTyping: &chatpb.IsTyping{
				MemberId: memberId.ToProto(),
				IsTyping: req.IsTyping,
			},
		},
	}

	if err := s.asyncNotifyAll(chatId, event); err != nil {
		log.WithError(err).Warn("failure notifying chat event")
	}

	return &chatpb.NotifyIsTypingResponse{}, nil
}

func (s *Server) NotifyMessage(_ context.Context, _ chat.ChatId, _ *chatpb.Message) {
	// TODO: Cleanup this up
}

// todo: needs to have a 'fill' version
func (s *Server) getMetadata(ctx context.Context, asMember chat.MemberId, id chat.ChatId) (*chatpb.Metadata, error) {
	mdRecord, err := s.data.GetChatMetadata(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup metadata: %w", err)
	}

	members, err := s.data.GetChatMembersV2(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get members: %w", err)
	}

	return s.populateMetadata(ctx, mdRecord, members, asMember)
}

func (s *Server) populateMetadata(ctx context.Context, mdRecord *chat.MetadataRecord, members []*chat.MemberRecord, asMember chat.MemberId) (*chatpb.Metadata, error) {
	md := &chatpb.Metadata{
		ChatId:    mdRecord.ChatId.ToProto(),
		Type:      mdRecord.ChatType.ToProto(),
		Cursor:    &chatpb.Cursor{Value: mdRecord.ChatId[:]},
		IsMuted:   false,
		Muteable:  false,
		NumUnread: 0,
	}

	if mdRecord.ChatTitle != nil {
		md.Title = *mdRecord.ChatTitle
	}

	for _, m := range members {
		memberId, err := chat.GetMemberIdFromString(m.MemberId)
		if err != nil {
			return nil, fmt.Errorf("invalid member id %q: %w", m.MemberId, err)
		}

		member := &chatpb.Member{
			MemberId: memberId.ToProto(),
		}
		md.Members = append(md.Members, member)

		twitterUser, err := s.data.GetTwitterUserByTipAddress(ctx, m.MemberId)
		if errors.Is(err, twitter.ErrUserNotFound) {
			s.log.WithField("member", m.MemberId).Info("Twitter user not found for existing user")
		} else if err != nil {
			// TODO: If client have caching, we could just not do this...
			return nil, fmt.Errorf("failed to get twitter user: %w", err)
		} else {
			member.Identity = &chatpb.MemberIdentity{
				Platform:      chatpb.Platform_TWITTER,
				Username:      twitterUser.Username,
				DisplayName:   twitterUser.Name,
				ProfilePicUrl: twitterUser.ProfilePicUrl,
			}
		}

		if m.DeliveryPointer != nil {
			member.Pointers = append(member.Pointers, &chatpb.Pointer{
				Type:     chatpb.PointerType_DELIVERED,
				Value:    m.DeliveryPointer.ToProto(),
				MemberId: memberId.ToProto(),
			})
		}
		if m.ReadPointer != nil {
			member.Pointers = append(member.Pointers, &chatpb.Pointer{
				Type:     chatpb.PointerType_READ,
				Value:    m.ReadPointer.ToProto(),
				MemberId: memberId.ToProto(),
			})
		}

		md.IsMuted = m.IsMuted

		// If the member is not the requestor, then we can skip further processing
		if !bytes.Equal(asMember, memberId) {
			continue
		}

		member.IsSelf = true

		// TODO: Do we actually want to compute this feature? It's maybe non-trivial.
		//       Maybe should have a safety valve at minimum.
		md.NumUnread, err = s.data.GetChatUnreadCountV2(ctx, mdRecord.ChatId, memberId, m.ReadPointer)
		if err != nil {
			return nil, fmt.Errorf("failed to get unread count: %w", err)
		}
	}

	return md, nil
}

func (s *Server) flushMessages(ctx context.Context, chatId chat.ChatId, owner *common.Account, stream *chatEventStream) {
	log := s.log.WithFields(logrus.Fields{
		"method":        "flushMessages",
		"chat_id":       chatId.String(),
		"owner_account": owner.PublicKey().ToBase58(),
	})

	protoChatMessages, err := s.getProtoChatMessages(
		ctx,
		chatId,
		owner,
		query.WithCursor(query.EmptyCursor),
		query.WithDirection(query.Descending),
		query.WithLimit(flushMessageCount),
	)
	if err != nil {
		log.WithError(err).Warn("failure getting chat messages")
		return
	}

	for _, protoChatMessage := range protoChatMessages {
		event := &chatpb.ChatStreamEvent{
			Type: &chatpb.ChatStreamEvent_Message{
				Message: protoChatMessage,
			},
		}
		if err := stream.notify(event, streamNotifyTimeout); err != nil {
			log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
			return
		}
	}
}

func (s *Server) flushPointers(ctx context.Context, chatId chat.ChatId, stream *chatEventStream) {
	log := s.log.WithFields(logrus.Fields{
		"method":  "flushPointers",
		"chat_id": chatId.String(),
	})

	memberRecords, err := s.data.GetChatMembersV2(ctx, chatId)
	if err != nil {
		log.WithError(err).Warn("failure getting chat members")
		return
	}

	for _, memberRecord := range memberRecords {
		for _, optionalPointer := range []struct {
			kind  chat.PointerType
			value *chat.MessageId
		}{
			{chat.PointerTypeDelivered, memberRecord.DeliveryPointer},
			{chat.PointerTypeRead, memberRecord.ReadPointer},
		} {
			if optionalPointer.value == nil {
				continue
			}

			memberId, err := chat.GetMemberIdFromString(memberRecord.MemberId)
			if err != nil {
				log.WithError(err).Warnf("failure getting memberId from %s", memberRecord.MemberId)
				return
			}

			event := &chatpb.ChatStreamEvent{
				Type: &chatpb.ChatStreamEvent_Pointer{
					Pointer: &chatpb.Pointer{
						Type:     optionalPointer.kind.ToProto(),
						Value:    optionalPointer.value.ToProto(),
						MemberId: memberId.ToProto(),
					},
				},
			}
			if err := stream.notify(event, streamNotifyTimeout); err != nil {
				log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
				return
			}
		}
	}
}

func (s *Server) getProtoChatMessages(ctx context.Context, chatId chat.ChatId, owner *common.Account, queryOptions ...query.Option) ([]*chatpb.Message, error) {
	messageRecords, err := s.data.GetAllChatMessagesV2(ctx, chatId, queryOptions...)
	if err != nil {
		return nil, fmt.Errorf("failure getting chat messages: %w", err)
	}

	var userLocale *language.Tag // Loaded lazily when required
	var res []*chatpb.Message
	for _, messageRecord := range messageRecords {
		var protoChatMessage chatpb.Message
		err = proto.Unmarshal(messageRecord.Payload, &protoChatMessage)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshalling proto chat message")
		}

		ts, err := messageRecord.GetTimestamp()
		if err != nil {
			return nil, errors.Wrap(err, "error getting message timestamp")
		}

		for _, content := range protoChatMessage.Content {
			switch typed := content.Type.(type) {
			case *chatpb.Content_Localized:
				if userLocale == nil {
					loadedUserLocale, err := s.data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
					if err != nil {
						return nil, errors.Wrap(err, "error getting user locale")
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

		protoChatMessage.MessageId = messageRecord.MessageId.ToProto()
		if messageRecord.Sender != nil {
			protoChatMessage.SenderId = messageRecord.Sender.ToProto()
		}
		protoChatMessage.Ts = timestamppb.New(ts)
		protoChatMessage.Cursor = &chatpb.Cursor{Value: messageRecord.MessageId[:]}

		res = append(res, &protoChatMessage)
	}

	return res, nil
}

// todo: This belongs in the common chat utility, which currently only operates on v1 chats
func (s *Server) persistChatMessage(ctx context.Context, chatId chat.ChatId, protoChatMessage *chatpb.Message) error {
	if err := protoChatMessage.Validate(); err != nil {
		return errors.Wrap(err, "proto chat message failed validation")
	}

	messageId, err := chat.GetMessageIdFromProto(protoChatMessage.MessageId)
	if err != nil {
		return errors.Wrap(err, "invalid message id")
	}

	var senderId *chat.MemberId
	if protoChatMessage.SenderId != nil {
		convertedSenderId, err := chat.GetMemberIdFromProto(protoChatMessage.SenderId)
		if err != nil {
			return errors.Wrap(err, "invalid member id")
		}
		senderId = &convertedSenderId
	}

	// Clear out extracted metadata as a space optimization
	cloned := proto.Clone(protoChatMessage).(*chatpb.Message)
	cloned.MessageId = nil
	cloned.SenderId = nil
	cloned.Ts = nil
	cloned.Cursor = nil

	marshalled, err := proto.Marshal(cloned)
	if err != nil {
		return errors.Wrap(err, "error marshalling proto chat message")
	}

	messageRecord := &chat.MessageRecord{
		ChatId:    chatId,
		MessageId: messageId,
		Sender:    senderId,
		Payload:   marshalled,
		IsSilent:  false,
	}

	err = s.data.PutChatMessageV2(ctx, messageRecord)
	if err != nil {
		return errors.Wrap(err, "error persisting chat message")
	}
	return nil
}

func (s *Server) onPersistChatMessage(log *logrus.Entry, chatId chat.ChatId, chatMessage *chatpb.Message) {
	event := &chatpb.ChatStreamEvent{
		Type: &chatpb.ChatStreamEvent_Message{
			Message: chatMessage,
		},
	}

	if err := s.asyncNotifyAll(chatId, event); err != nil {
		log.WithError(err).Warn("failure notifying chat event")
	}
}

func (s *Server) sendPushNotifications(chatId chat.ChatId, chatTitle string, sender chat.MemberId, message *chatpb.Message) {
	log := s.log.WithFields(logrus.Fields{
		"method":  "sendPushNotifications",
		"sender":  sender.String(),
		"chat_id": chatId.String(),
	})

	// TODO: Callers might already have this loaded.
	members, err := s.data.GetChatMembersV2(context.Background(), chatId)
	if err != nil {
		log.WithError(err).Warn("failure getting chat members")
		return
	}

	var eg errgroup.Group
	eg.SetLimit(min(32, len(members)))

	for _, m := range members {
		if m.Owner == "" || m.IsMuted {
			continue
		}

		owner, err := common.NewAccountFromPublicKeyString(m.Owner)
		if err != nil {
			log.WithError(err).WithField("member", m.MemberId).Warn("failure getting owner")
			continue
		}

		m := m
		eg.Go(func() error {
			log.WithField("member", m.MemberId).Info("sending push notification")
			err = push_util.SendChatMessagePushNotificationV2(
				context.Background(),
				s.data,
				s.push,
				chatId,
				chatTitle,
				owner,
				message,
			)
			if err != nil {
				log.
					WithError(err).
					WithField("member", m.MemberId).
					Warn("failure sending push notification")
			}

			return nil
		})
	}

	_ = eg.Wait()
}

func newProtoChatMessage(sender chat.MemberId, content ...*chatpb.Content) *chatpb.Message {
	messageId := chat.GenerateMessageId()
	ts, _ := messageId.GetTimestamp()

	return &chatpb.Message{
		MessageId: messageId.ToProto(),
		SenderId:  sender.ToProto(),
		Content:   content,
		Ts:        timestamppb.New(ts),
		Cursor:    &chatpb.Cursor{Value: messageId[:]},
	}
}
