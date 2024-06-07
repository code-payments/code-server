package chat_v2

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/grpc/client"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

// todo: Ensure all relevant logging fields are set
type server struct {
	log *logrus.Entry

	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	chatpb.UnimplementedChatServer
}

func NewChatServer(data code_data.Provider, auth *auth_util.RPCSignatureVerifier) chatpb.ChatServer {
	return &server{
		log:  logrus.StandardLogger().WithField("type", "chat/v2/server"),
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

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	return nil, status.Error(codes.Unimplemented, "")
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

	if req.GetOpenStream().Signature == nil {
		return status.Error(codes.InvalidArgument, "signature is nil")
	}

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

	return status.Error(codes.Unimplemented, "")
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

	isChatMember, err := s.ownsChatMember(ctx, chatId, memberId, owner)
	if err != nil {
		log.WithError(err).Warn("failure determing chat member ownership")
		return nil, status.Error(codes.Internal, "")
	} else if !isChatMember {
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
