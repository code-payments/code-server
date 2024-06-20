package chat_v2

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"
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
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/localization"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

// todo: resolve some common code for sending chat messages across RPCs

const (
	maxGetChatsPageSize    = 100
	maxGetMessagesPageSize = 100
	flushMessageCount      = 100
)

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

// todo: This will require a lot of optimizations since we iterate and make several DB calls for each chat membership
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
	} else {
		cursor = query.ToCursor(0)
		if direction == query.Descending {
			cursor = query.ToCursor(math.MaxInt64 - 1)
		}
	}

	myIdentities, err := s.getAllIdentities(ctx, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting identities for owner account")
		return nil, status.Error(codes.Internal, "")
	}

	// todo: Use a better query that returns chat IDs. This will result in duplicate
	//       chat results if the user is in the chat multiple times across many identities.
	patformUserMemberRecords, err := s.data.GetPlatformUserChatMembershipV2(
		ctx,
		myIdentities,
		query.WithCursor(cursor),
		query.WithDirection(direction),
		query.WithLimit(limit),
	)
	if err == chat.ErrMemberNotFound {
		return &chatpb.GetChatsResponse{
			Result: chatpb.GetChatsResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat members for platform user")
		return nil, status.Error(codes.Internal, "")
	}

	var protoChats []*chatpb.ChatMetadata
	for _, platformUserMemberRecord := range patformUserMemberRecords {
		log := log.WithField("chat_id", platformUserMemberRecord.ChatId.String())

		chatRecord, err := s.data.GetChatByIdV2(ctx, platformUserMemberRecord.ChatId)
		if err != nil {
			log.WithError(err).Warn("failure getting chat record")
			return nil, status.Error(codes.Internal, "")
		}

		memberRecords, err := s.data.GetAllChatMembersV2(ctx, chatRecord.ChatId)
		if err != nil {
			log.WithError(err).Warn("failure getting chat members")
			return nil, status.Error(codes.Internal, "")
		}

		protoChat, err := s.toProtoChat(ctx, chatRecord, memberRecords, myIdentities)
		if err != nil {
			log.WithError(err).Warn("failure constructing proto chat message")
			return nil, status.Error(codes.Internal, "")
		}
		protoChat.Cursor = &chatpb.Cursor{Value: query.ToCursor(uint64(platformUserMemberRecord.Id))}

		protoChats = append(protoChats, protoChat)
	}

	return &chatpb.GetChatsResponse{
		Result: chatpb.GetChatsResponse_OK,
		Chats:  protoChats,
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
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
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
	if err == chat.ErrMessageNotFound {
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_MESSAGE_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting chat messages")
		return nil, status.Error(codes.Internal, "")
	}

	if len(protoChatMessages) == 0 {
		return &chatpb.GetMessagesResponse{
			Result: chatpb.GetMessagesResponse_MESSAGE_NOT_FOUND,
		}, nil
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

	memberId, err := chat.GetMemberIdFromProto(req.GetOpenStream().MemberId)
	if err != nil {
		log.WithError(err).Warn("invalid member id")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.GetOpenStream().Signature
	req.GetOpenStream().Signature = nil
	if err := s.auth.Authenticate(streamer.Context(), owner, req.GetOpenStream(), signature); err != nil {
		return err
	}

	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return streamer.Send(&chatpb.StreamChatEventsResponse{
			Type: &chatpb.StreamChatEventsResponse_Error{
				Error: &chatpb.ChatStreamEventError{Code: chatpb.ChatStreamEventError_CHAT_NOT_FOUND},
			},
		})
	default:
		log.WithError(err).Warn("failure getting chat record")
		return status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return status.Error(codes.Internal, "")
	} else if !ownsChatMember {
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

	go s.flushMessages(ctx, chatId, owner, stream)
	go s.flushPointers(ctx, chatId, stream)

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

func (s *server) flushMessages(ctx context.Context, chatId chat.ChatId, owner *common.Account, stream *chatEventStream) {
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
	if err == chat.ErrMessageNotFound {
		return
	} else if err != nil {
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

func (s *server) flushPointers(ctx context.Context, chatId chat.ChatId, stream *chatEventStream) {
	log := s.log.WithFields(logrus.Fields{
		"method":  "flushPointers",
		"chat_id": chatId.String(),
	})

	memberRecords, err := s.data.GetAllChatMembersV2(ctx, chatId)
	if err == chat.ErrMemberNotFound {
		return
	} else if err != nil {
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

			event := &chatpb.ChatStreamEvent{
				Type: &chatpb.ChatStreamEvent_Pointer{
					Pointer: &chatpb.Pointer{
						Type:     optionalPointer.kind.ToProto(),
						Value:    optionalPointer.value.ToProto(),
						MemberId: memberRecord.MemberId.ToProto(),
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

func (s *server) StartChat(ctx context.Context, req *chatpb.StartChatRequest) (*chatpb.StartChatResponse, error) {
	log := s.log.WithField("method", "StartChat")
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

	switch typed := req.Parameters.(type) {
	case *chatpb.StartChatRequest_TipChat:
		intentId := base58.Encode(typed.TipChat.IntentId.Value)
		log = log.WithField("intent", intentId)

		intentRecord, err := s.data.GetIntent(ctx, intentId)
		if err == intent.ErrIntentNotFound {
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_INVALID_PARAMETER,
				Chat:   nil,
			}, nil
		} else if err != nil {
			log.WithError(err).Warn("failure getting intent record")
			return nil, status.Error(codes.Internal, "")
		}

		// The intent was not for a tip.
		if intentRecord.SendPrivatePaymentMetadata == nil || !intentRecord.SendPrivatePaymentMetadata.IsTip {
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_INVALID_PARAMETER,
				Chat:   nil,
			}, nil
		}

		tipper, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
		if err != nil {
			log.WithError(err).Warn("invalid tipper owner account")
			return nil, status.Error(codes.Internal, "")
		}
		log = log.WithField("tipper", tipper.PublicKey().ToBase58())

		tippee, err := common.NewAccountFromPublicKeyString(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount)
		if err != nil {
			log.WithError(err).Warn("invalid tippee owner account")
			return nil, status.Error(codes.Internal, "")
		}
		log = log.WithField("tippee", tippee.PublicKey().ToBase58())

		// For now, don't allow chats where you tipped yourself.
		//
		// todo: How do we want to handle this case?
		if owner.PublicKey().ToBase58() == tipper.PublicKey().ToBase58() {
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_INVALID_PARAMETER,
				Chat:   nil,
			}, nil
		}

		// Only the owner of the platform user at the time of tipping can initiate the chat.
		if owner.PublicKey().ToBase58() != tippee.PublicKey().ToBase58() {
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_DENIED,
				Chat:   nil,
			}, nil
		}

		// todo: This will require a refactor when we allow creation of other types of chats
		switch intentRecord.SendPrivatePaymentMetadata.TipMetadata.Platform {
		case transactionpb.TippedUser_TWITTER:
			twitterUsername := intentRecord.SendPrivatePaymentMetadata.TipMetadata.Username

			// The owner must still own the Twitter username
			ownsUsername, err := s.ownsTwitterUsername(ctx, owner, twitterUsername)
			if err != nil {
				log.WithError(err).Warn("failure determing twitter username ownership")
				return nil, status.Error(codes.Internal, "")
			} else if !ownsUsername {
				return &chatpb.StartChatResponse{
					Result: chatpb.StartChatResponse_DENIED,
				}, nil
			}

			// todo: try to find an existing chat, but for now always create a new completely random one
			var chatId chat.ChatId
			rand.Read(chatId[:])

			creationTs := time.Now()

			chatRecord := &chat.ChatRecord{
				ChatId:   chatId,
				ChatType: chat.ChatTypeTwoWay,

				IsVerified: true,

				CreatedAt: creationTs,
			}

			memberRecords := []*chat.MemberRecord{
				{
					ChatId:   chatId,
					MemberId: chat.GenerateMemberId(),

					Platform:   chat.PlatformTwitter,
					PlatformId: twitterUsername,

					JoinedAt: creationTs,
				},
				{
					ChatId:   chatId,
					MemberId: chat.GenerateMemberId(),

					Platform:   chat.PlatformCode,
					PlatformId: tipper.PublicKey().ToBase58(),

					JoinedAt: creationTs,
				},
			}

			err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
				err := s.data.PutChatV2(ctx, chatRecord)
				if err != nil {
					return errors.Wrap(err, "error creating chat record")
				}

				for _, memberRecord := range memberRecords {
					err := s.data.PutChatMemberV2(ctx, memberRecord)
					if err != nil {
						return errors.Wrap(err, "error creating member record")
					}
				}

				return nil
			})
			if err != nil {
				log.WithError(err).Warn("failure creating new chat")
				return nil, status.Error(codes.Internal, "")
			}

			protoChat, err := s.toProtoChat(
				ctx,
				chatRecord,
				memberRecords,
				map[chat.Platform]string{
					chat.PlatformCode:    owner.PublicKey().ToBase58(),
					chat.PlatformTwitter: twitterUsername,
				},
			)
			if err != nil {
				log.WithError(err).Warn("failure constructing proto chat message")
				return nil, status.Error(codes.Internal, "")
			}

			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_OK,
				Chat:   protoChat,
			}, nil
		default:
			return &chatpb.StartChatResponse{
				Result: chatpb.StartChatResponse_INVALID_PARAMETER,
				Chat:   nil,
			}, nil
		}

	default:
		return nil, status.Error(codes.InvalidArgument, "StartChatRequest.Parameters is nil")
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

	switch req.Content[0].Type.(type) {
	case *chatpb.Content_Text, *chatpb.Content_ThankYou:
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

	chatRecord, err := s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return &chatpb.SendMessageResponse{
			Result: chatpb.SendMessageResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	switch chatRecord.ChatType {
	case chat.ChatTypeTwoWay:
	default:
		return &chatpb.SendMessageResponse{
			Result: chatpb.SendMessageResponse_INVALID_CHAT_TYPE,
		}, nil
	}

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
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

	return &chatpb.SendMessageResponse{
		Result:  chatpb.SendMessageResponse_OK,
		Message: chatMessage,
	}, nil
}

// todo: This belongs in the common chat utility, which currently only operates on v1 chats
func (s *server) persistChatMessage(ctx context.Context, chatId chat.ChatId, protoChatMessage *chatpb.ChatMessage) error {
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
	cloned := proto.Clone(protoChatMessage).(*chatpb.ChatMessage)
	cloned.MessageId = nil
	cloned.SenderId = nil
	cloned.Ts = nil
	cloned.Cursor = nil

	marshalled, err := proto.Marshal(cloned)
	if err != nil {
		return errors.Wrap(err, "error marshalling proto chat message")
	}

	// todo: Doesn't incoroporate reference. We might want to promote the proto a level above the content.
	messageRecord := &chat.MessageRecord{
		ChatId:    chatId,
		MessageId: messageId,

		Sender: senderId,

		Data: marshalled,

		IsSilent: false,
	}

	err = s.data.PutChatMessageV2(ctx, messageRecord)
	if err != nil {
		return errors.Wrap(err, "error persiting chat message")
	}
	return nil
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

	pointerType := chat.GetPointerTypeFromProto(req.Pointer.Type)
	log = log.WithField("pointer_type", pointerType.String())
	switch pointerType {
	case chat.PointerTypeDelivered, chat.PointerTypeRead:
	default:
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_INVALID_POINTER_TYPE,
		}, nil
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

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return &chatpb.AdvancePointerResponse{
			Result: chatpb.AdvancePointerResponse_DENIED,
		}, nil
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

func (s *server) RevealIdentity(ctx context.Context, req *chatpb.RevealIdentityRequest) (*chatpb.RevealIdentityResponse, error) {
	log := s.log.WithField("method", "RevealIdentity")
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

	platform := chat.GetPlatformFromProto(req.Identity.Platform)

	log = log.WithFields(logrus.Fields{
		"platform": platform.String(),
		"username": req.Identity.Username,
	})

	_, err = s.data.GetChatByIdV2(ctx, chatId)
	switch err {
	case nil:
	case chat.ErrChatNotFound:
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_CHAT_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting chat record")
		return nil, status.Error(codes.Internal, "")
	}

	memberRecord, err := s.data.GetChatMemberByIdV2(ctx, chatId, memberId)
	switch err {
	case nil:
	case chat.ErrMemberNotFound:
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_DENIED,
		}, nil
	default:
		log.WithError(err).Warn("failure getting member record")
		return nil, status.Error(codes.Internal, "")
	}

	ownsChatMember, err := s.ownsChatMemberWithRecord(ctx, chatId, memberRecord, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_DENIED,
		}, nil
	}

	switch platform {
	case chat.PlatformTwitter:
		ownsUsername, err := s.ownsTwitterUsername(ctx, owner, req.Identity.Username)
		if err != nil {
			log.WithError(err).Warn("failure determing twitter username ownership")
			return nil, status.Error(codes.Internal, "")
		} else if !ownsUsername {
			return &chatpb.RevealIdentityResponse{
				Result: chatpb.RevealIdentityResponse_DENIED,
			}, nil
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "RevealIdentityRequest.Identity.Platform must be TWITTER")
	}

	// Idempotent RPC call using the same platform and username
	if memberRecord.Platform == platform && memberRecord.PlatformId == req.Identity.Username {
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_OK,
		}, nil
	}

	// Identity was already revealed, and it isn't the specified platform and username
	if memberRecord.Platform != chat.PlatformCode {
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_DIFFERENT_IDENTITY_REVEALED,
		}, nil
	}

	chatLock := s.chatLocks.Get(chatId[:])
	chatLock.Lock()
	defer chatLock.Unlock()

	chatMessage := newProtoChatMessage(
		memberId,
		&chatpb.Content{
			Type: &chatpb.Content_IdentityRevealed{
				IdentityRevealed: &chatpb.IdentityRevealedContent{
					MemberId: req.MemberId,
					Identity: req.Identity,
				},
			},
		},
	)

	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err = s.data.UpgradeChatMemberIdentityV2(ctx, chatId, memberId, platform, req.Identity.Username)
		switch err {
		case nil:
		case chat.ErrMemberIdentityAlreadyUpgraded:
			return err
		default:
			return errors.Wrap(err, "error updating chat member identity")
		}

		err := s.persistChatMessage(ctx, chatId, chatMessage)
		if err != nil {
			return errors.Wrap(err, "error persisting chat message")
		}
		return nil
	})

	if err == nil {
		s.onPersistChatMessage(log, chatId, chatMessage)
	}

	switch err {
	case nil:
		return &chatpb.RevealIdentityResponse{
			Result:  chatpb.RevealIdentityResponse_OK,
			Message: chatMessage,
		}, nil
	case chat.ErrMemberIdentityAlreadyUpgraded:
		return &chatpb.RevealIdentityResponse{
			Result: chatpb.RevealIdentityResponse_DIFFERENT_IDENTITY_REVEALED,
		}, nil
	default:
		log.WithError(err).Warn("failure upgrading chat member identity")
		return nil, status.Error(codes.Internal, "")
	}
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

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return &chatpb.SetMuteStateResponse{
			Result: chatpb.SetMuteStateResponse_DENIED,
		}, nil
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

	// todo: Use chat record to determine if unsubscribing is allowed
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

	ownsChatMember, err := s.ownsChatMemberWithoutRecord(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !ownsChatMember {
		return &chatpb.SetSubscriptionStateResponse{
			Result: chatpb.SetSubscriptionStateResponse_DENIED,
		}, nil
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

func (s *server) toProtoChat(ctx context.Context, chatRecord *chat.ChatRecord, memberRecords []*chat.MemberRecord, myIdentitiesByPlatform map[chat.Platform]string) (*chatpb.ChatMetadata, error) {
	protoChat := &chatpb.ChatMetadata{
		ChatId: chatRecord.ChatId.ToProto(),
		Type:   chatRecord.ChatType.ToProto(),
		Cursor: &chatpb.Cursor{Value: query.ToCursor(uint64(chatRecord.Id))},
	}

	switch chatRecord.ChatType {
	case chat.ChatTypeTwoWay:
		protoChat.Title = "Tip Chat" // todo: proper title with localization

		protoChat.CanMute = true
		protoChat.CanUnsubscribe = true
	default:
		return nil, errors.Errorf("unsupported chat type: %s", chatRecord.ChatType.String())
	}

	for _, memberRecord := range memberRecords {
		var isSelf bool
		var identity *chatpb.ChatMemberIdentity
		switch memberRecord.Platform {
		case chat.PlatformCode:
			myPublicKey, ok := myIdentitiesByPlatform[chat.PlatformCode]
			isSelf = ok && myPublicKey == memberRecord.PlatformId
		case chat.PlatformTwitter:
			myTwitterUsername, ok := myIdentitiesByPlatform[chat.PlatformTwitter]
			isSelf = ok && myTwitterUsername == memberRecord.PlatformId

			identity = &chatpb.ChatMemberIdentity{
				Platform: memberRecord.Platform.ToProto(),
				Username: memberRecord.PlatformId,
			}
		default:
			return nil, errors.Errorf("unsupported platform type: %s", memberRecord.Platform.String())
		}

		var pointers []*chatpb.Pointer
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

			pointers = append(pointers, &chatpb.Pointer{
				Type:     optionalPointer.kind.ToProto(),
				Value:    optionalPointer.value.ToProto(),
				MemberId: memberRecord.MemberId.ToProto(),
			})
		}

		protoMember := &chatpb.ChatMember{
			MemberId: memberRecord.MemberId.ToProto(),
			IsSelf:   isSelf,
			Identity: identity,
			Pointers: pointers,
		}
		if protoMember.IsSelf {
			protoMember.IsMuted = memberRecord.IsMuted
			protoMember.IsSubscribed = !memberRecord.IsUnsubscribed

			if !memberRecord.IsUnsubscribed {
				readPointer := chat.GenerateMessageIdAtTime(time.Unix(0, 0))
				if memberRecord.ReadPointer != nil {
					readPointer = *memberRecord.ReadPointer
				}
				unreadCount, err := s.data.GetChatUnreadCountV2(ctx, chatRecord.ChatId, memberRecord.MemberId, readPointer)
				if err != nil {
					return nil, errors.Wrap(err, "error calculating unread count")
				}
				protoMember.NumUnread = unreadCount
			}
		}

		protoChat.Members = append(protoChat.Members, protoMember)
	}

	return protoChat, nil
}

func (s *server) getProtoChatMessages(ctx context.Context, chatId chat.ChatId, owner *common.Account, queryOptions ...query.Option) ([]*chatpb.ChatMessage, error) {
	messageRecords, err := s.data.GetAllChatMessagesV2(
		ctx,
		chatId,
		queryOptions...,
	)
	if err == chat.ErrMessageNotFound {
		return nil, err
	}

	var userLocale *language.Tag // Loaded lazily when required
	var res []*chatpb.ChatMessage
	for _, messageRecord := range messageRecords {
		var protoChatMessage chatpb.ChatMessage
		err = proto.Unmarshal(messageRecord.Data, &protoChatMessage)
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

func (s *server) onPersistChatMessage(log *logrus.Entry, chatId chat.ChatId, chatMessage *chatpb.ChatMessage) {
	event := &chatpb.ChatStreamEvent{
		Type: &chatpb.ChatStreamEvent_Message{
			Message: chatMessage,
		},
	}
	if err := s.asyncNotifyAll(chatId, event); err != nil {
		log.WithError(err).Warn("failure notifying chat event")
	}

	// todo: send the push
}

func (s *server) getAllIdentities(ctx context.Context, owner *common.Account) (map[chat.Platform]string, error) {
	identities := map[chat.Platform]string{
		chat.PlatformCode: owner.PublicKey().ToBase58(),
	}

	twitterUserame, ok, err := s.getOwnedTwitterUsername(ctx, owner)
	if err != nil {
		return nil, err
	}
	if ok {
		identities[chat.PlatformTwitter] = twitterUserame
	}

	return identities, nil
}

func (s *server) ownsChatMemberWithoutRecord(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, owner *common.Account) (bool, error) {
	memberRecord, err := s.data.GetChatMemberByIdV2(ctx, chatId, memberId)
	switch err {
	case nil:
	case chat.ErrMemberNotFound:
		return false, nil
	default:
		return false, errors.Wrap(err, "error getting member record")
	}

	return s.ownsChatMemberWithRecord(ctx, chatId, memberRecord, owner)
}

func (s *server) ownsChatMemberWithRecord(ctx context.Context, chatId chat.ChatId, memberRecord *chat.MemberRecord, owner *common.Account) (bool, error) {
	switch memberRecord.Platform {
	case chat.PlatformCode:
		return memberRecord.PlatformId == owner.PublicKey().ToBase58(), nil
	case chat.PlatformTwitter:
		return s.ownsTwitterUsername(ctx, owner, memberRecord.PlatformId)
	default:
		return false, nil
	}
}

// todo: This logic should live elsewhere in somewhere more common
func (s *server) ownsTwitterUsername(ctx context.Context, owner *common.Account, username string) (bool, error) {
	ownerTipAccount, err := owner.ToTimelockVault(timelock_token.DataVersion1, common.KinMintAccount)
	if err != nil {
		return false, errors.Wrap(err, "error deriving twitter tip address")
	}

	twitterRecord, err := s.data.GetTwitterUserByUsername(ctx, username)
	switch err {
	case nil:
	case twitter.ErrUserNotFound:
		return false, nil
	default:
		return false, errors.Wrap(err, "error getting twitter user")
	}

	return twitterRecord.TipAddress == ownerTipAccount.PublicKey().ToBase58(), nil
}

// todo: This logic should live elsewhere in somewhere more common
func (s *server) getOwnedTwitterUsername(ctx context.Context, owner *common.Account) (string, bool, error) {
	ownerTipAccount, err := owner.ToTimelockVault(timelock_token.DataVersion1, common.KinMintAccount)
	if err != nil {
		return "", false, errors.Wrap(err, "error deriving twitter tip address")
	}

	twitterRecord, err := s.data.GetTwitterUserByTipAddress(ctx, ownerTipAccount.PublicKey().ToBase58())
	switch err {
	case nil:
		return twitterRecord.Username, true, nil
	case twitter.ErrUserNotFound:
		return "", false, nil
	default:
		return "", false, errors.Wrap(err, "error getting twitter user")
	}
}

func newProtoChatMessage(sender chat.MemberId, content ...*chatpb.Content) *chatpb.ChatMessage {
	messageId := chat.GenerateMessageId()
	ts, _ := messageId.GetTimestamp()

	return &chatpb.ChatMessage{
		MessageId: messageId.ToProto(),
		SenderId:  sender.ToProto(),
		Content:   content,
		Ts:        timestamppb.New(ts),
		Cursor:    &chatpb.Cursor{Value: messageId[:]},
	}
}
