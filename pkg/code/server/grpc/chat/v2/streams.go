package chat_v2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

func (s *Server) StreamMessages(stream chatpb.Chat_StreamMessagesServer) error {
	ctx := stream.Context()
	log := s.log.WithField("method", "StreamMessages")
	log = client.InjectLoggingMetadata(ctx, log)

	req, err := boundedReceive[chatpb.StreamMessagesRequest, *chatpb.StreamMessagesRequest](
		ctx,
		stream,
		250*time.Millisecond,
	)
	if err != nil {
		return err
	}

	if req.GetParams() == nil {
		return status.Error(codes.InvalidArgument, "StreamChatEventsRequest.Type must be OpenStreamRequest")
	}

	owner, err := common.NewAccountFromProto(req.GetParams().Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	chatId, err := chat.GetChatIdFromProto(req.GetParams().ChatId)
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

	signature := req.GetParams().Signature
	req.GetParams().Signature = nil
	if err := s.auth.Authenticate(stream.Context(), owner, req.GetParams(), signature); err != nil {
		return err
	}

	isMember, err := s.data.IsChatMember(ctx, chatId, memberId)
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return status.Error(codes.Internal, "")
	}
	if !isMember {
		return stream.Send(&chatpb.StreamMessagesResponse{
			Type: &chatpb.StreamMessagesResponse_Error{
				Error: &chatpb.StreamError{Code: chatpb.StreamError_DENIED},
			},
		})
	}

	streamKey := fmt.Sprintf("%s:%s", chatId.String(), memberId.String())

	s.streamsMu.Lock()

	if _, exists := s.streams[streamKey]; exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this Server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this Server (stream=%p) ; aborting", stream)
		return status.Error(codes.Aborted, "stream already exists")
	}

	ss := newEventStream[*chatpb.StreamMessagesResponse_MessageBatch](
		streamBufferSize,
		func(notification *chatEventNotification) (*chatpb.StreamMessagesResponse_MessageBatch, bool) {
			if notification.messageUpdate == nil {
				return nil, false
			}
			if notification.chatId != chatId {
				return nil, false
			}

			return &chatpb.StreamMessagesResponse_MessageBatch{
				Messages: []*chatpb.Message{notification.messageUpdate},
			}, true
		},
	)

	// The race detector complains when reading the stream pointer ref outside of the lock.
	streamRef := fmt.Sprintf("%p", stream)
	log.Tracef("setting up new stream (stream=%s)", streamRef)
	s.streams[streamKey] = ss

	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Tracef("closing streamer (stream=%s)", streamRef)

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamChatEvents() call is handling it.
		liveStream, exists := s.streams[streamKey]
		if exists && liveStream == ss {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()
	}()

	sendPingCh := time.After(0)
	streamHealthCh := monitorStreamHealth(ctx, log, streamRef, stream, func(t *chatpb.StreamMessagesRequest) bool {
		return t.GetPong() != nil
	})

	// TODO: Support pagination options (or just remove if not necessary).
	go s.flushMessages(ctx, chatId, owner, ss)
	go s.flushPointers(ctx, chatId, owner, ss)

	for {
		select {
		case batch, ok := <-ss.ch:
			if !ok {
				log.Tracef("stream closed ; ending stream (stream=%s)", streamRef)
				return status.Error(codes.Aborted, "stream closed")
			}

			resp := &chatpb.StreamMessagesResponse{
				Type: &chatpb.StreamMessagesResponse_Messages{
					Messages: batch,
				},
			}

			if err = stream.Send(resp); err != nil {
				log.WithError(err).Info("failed to forward chat message")
				return err
			}
		case <-sendPingCh:
			log.Tracef("sending ping to client (stream=%s)", streamRef)

			sendPingCh = time.After(streamPingDelay)

			err := stream.Send(&chatpb.StreamMessagesResponse{
				Type: &chatpb.StreamMessagesResponse_Ping{
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

func (s *Server) StreamChatEvents(stream chatpb.Chat_StreamChatEventsServer) error {
	ctx := stream.Context()
	log := s.log.WithField("method", "StreamChatEvents")
	log = client.InjectLoggingMetadata(ctx, log)

	req, err := boundedReceive[chatpb.StreamChatEventsRequest, *chatpb.StreamChatEventsRequest](
		ctx,
		stream,
		250*time.Millisecond,
	)
	if err != nil {
		return err
	}

	if req.GetParams() == nil {
		return status.Error(codes.InvalidArgument, "StreamChatEventsRequest.Type must be OpenStreamRequest")
	}

	owner, err := common.NewAccountFromProto(req.GetParams().Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	memberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failed to derive messaging account")
		return status.Error(codes.Internal, "")
	}
	log = log.WithField("member_id", memberId.String())

	signature := req.GetParams().Signature
	req.GetParams().Signature = nil
	if err := s.auth.Authenticate(stream.Context(), owner, req.GetParams(), signature); err != nil {
		return err
	}

	// This should be safe? The user would have to provide a pub key
	// that derives to a collision on another stream key (i.e. messages)
	streamKey := fmt.Sprintf("%s", memberId.String())

	s.streamsMu.Lock()

	if _, exists := s.streams[streamKey]; exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this Server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this Server (stream=%p) ; aborting", stream)
		return status.Error(codes.Aborted, "stream already exists")
	}

	ss := newEventStream[*chatpb.StreamChatEventsResponse_EventBatch](
		streamBufferSize,
		func(notification *chatEventNotification) (*chatpb.StreamChatEventsResponse_EventBatch, bool) {
			// We need to check memberships here.
			//
			// TODO: This needs to be heavily cached
			isMember, err := s.data.IsChatMember(ctx, notification.chatId, memberId)
			if err != nil {
				log.Warn("Failed to check if member for notification", zap.String("chat_id", notification.chatId.String()))
			} else if !isMember {
				log.Debug("Notification for chat not a member of, dropping", zap.String("chat_id", notification.chatId.String()))
				return nil, false
			}

			return &chatpb.StreamChatEventsResponse_EventBatch{
				Updates: []*chatpb.StreamChatEventsResponse_ChatUpdate{
					{
						ChatId:      notification.chatId.ToProto(),
						Metadata:    notification.chatUpdate,
						LastMessage: notification.messageUpdate,
						Pointer:     notification.pointerUpdate,
						IsTyping:    notification.isTyping,
					},
				},
			}, true
		},
	)

	// The race detector complains when reading the stream pointer ref outside of the lock.
	streamRef := fmt.Sprintf("%p", stream)
	log.Tracef("setting up new stream (stream=%s)", streamRef)
	s.streams[streamKey] = ss

	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Tracef("closing streamer (stream=%s)", streamRef)

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamChatEvents() call is handling it.
		liveStream, exists := s.streams[streamKey]
		if exists && liveStream == ss {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()
	}()

	sendPingCh := time.After(0)
	streamHealthCh := monitorStreamHealth(ctx, log, streamRef, stream, func(t *chatpb.StreamMessagesRequest) bool {
		return t.GetPong() != nil
	})

	go s.flushChats(ctx, owner, memberId, ss)

	for {
		select {
		case batch, ok := <-ss.ch:
			if !ok {
				log.Tracef("stream closed ; ending stream (stream=%s)", streamRef)
				return status.Error(codes.Aborted, "stream closed")
			}

			resp := &chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Events{
					Events: batch,
				},
			}

			if err = stream.Send(resp); err != nil {
				log.WithError(err).Info("failed to forward chat message")
				return err
			}
		case <-sendPingCh:
			log.Tracef("sending ping to client (stream=%s)", streamRef)

			sendPingCh = time.After(streamPingDelay)

			err := stream.Send(&chatpb.StreamChatEventsResponse{
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

func (s *Server) flushMessages(ctx context.Context, chatId chat.ChatId, owner *common.Account, stream eventStream) {
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
		event := &chatEventNotification{
			chatId:        chatId,
			messageUpdate: protoChatMessage,
		}

		if err = stream.notify(event, streamNotifyTimeout); err != nil {
			log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
			return
		}
	}
}

func (s *Server) flushPointers(ctx context.Context, chatId chat.ChatId, owner *common.Account, stream eventStream) {
	log := s.log.WithFields(logrus.Fields{
		"method":  "flushPointers",
		"chat_id": chatId.String(),
	})

	callingMemberId, err := owner.ToChatMemberId()
	if err != nil {
		log.WithError(err).Warn("failure computing self")
		return
	}

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

			if bytes.Equal(memberId, callingMemberId) {
				continue
			}

			event := &chatEventNotification{
				pointerUpdate: &chatpb.Pointer{
					Type:     optionalPointer.kind.ToProto(),
					Value:    optionalPointer.value.ToProto(),
					MemberId: memberId.ToProto(),
				},
			}
			if err := stream.notify(event, streamNotifyTimeout); err != nil {
				log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
				return
			}
		}
	}
}

func (s *Server) flushChats(ctx context.Context, owner *common.Account, memberId chat.MemberId, stream eventStream) {
	log := s.log.WithFields(logrus.Fields{
		"method":    "flushChats",
		"member_id": memberId.String(),
	})

	chats, err := s.data.GetAllChatsForUserV2(ctx, memberId)
	if err != nil {
		log.WithError(err).Warn("failed get chats")
		return
	}

	// TODO: This needs to be far safer.
	for _, chatId := range chats {
		go func(chatId chat.ChatId) {
			md, err := s.getMetadata(ctx, memberId, chatId)
			if err != nil {
				log.WithError(err).Warn("failed get metadata", zap.String("chat_id", chatId.String()))
				return
			}

			messages, err := s.getProtoChatMessages(
				ctx,
				chatId,
				owner,
				query.WithLimit(1),
				query.WithDirection(query.Descending),
			)
			if err != nil {
				log.WithError(err).Warn("failed get chat messages", zap.String("chat_id", chatId.String()))
			}

			event := &chatEventNotification{
				chatId:     chatId,
				chatUpdate: md,
			}
			if len(messages) > 0 {
				event.messageUpdate = messages[0]
			}
		}(chatId)
	}
}

type chatEventNotification struct {
	chatId chat.ChatId
	ts     time.Time

	chatUpdate    *chatpb.Metadata
	pointerUpdate *chatpb.Pointer
	messageUpdate *chatpb.Message
	isTyping      *chatpb.IsTyping
}

func (s *Server) asyncNotifyAll(chatId chat.ChatId, event *chatEventNotification) error {
	event.ts = time.Now()
	ok := s.chatEventChans.Send(chatId[:], event)
	if !ok {
		return errors.New("chat event channel is full")
	}

	return nil
}

func (s *Server) asyncChatEventStreamNotifier(workerId int, channel <-chan any) {
	log := s.log.WithFields(logrus.Fields{
		"method": "asyncChatEventStreamNotifier",
		"worker": workerId,
	})

	for value := range channel {
		typedValue, ok := value.(*chatEventNotification)
		if !ok {
			log.Warn("channel did not receive expected struct")
			continue
		}

		log = log.WithField("chat_id", typedValue.chatId.String())

		if time.Since(typedValue.ts) > time.Second {
			log.Warn("channel notification latency is elevated")
		}

		s.streamsMu.RLock()
		for _, stream := range s.streams {
			if err := stream.notify(typedValue, streamNotifyTimeout); err != nil {
				log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
			}
		}
		s.streamsMu.RUnlock()
	}
}
