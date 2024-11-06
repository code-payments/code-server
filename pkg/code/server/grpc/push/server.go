package push

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pushpb "github.com/code-payments/code-protobuf-api/generated/go/push/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

type pushServer struct {
	log          *logrus.Entry
	data         code_data.Provider
	auth         *auth_util.RPCSignatureVerifier
	pushProvider push_lib.Provider

	pushpb.UnimplementedPushServer
}

func NewPushServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
	pushProvider push_lib.Provider,
) pushpb.PushServer {
	return &pushServer{
		log:          logrus.StandardLogger().WithField("type", "push/server"),
		data:         data,
		auth:         auth,
		pushProvider: pushProvider,
	}
}

func (s *pushServer) AddToken(ctx context.Context, req *pushpb.AddTokenRequest) (*pushpb.AddTokenResponse, error) {
	/*
		log := s.log.WithField("method", "AddToken")
		log = client.InjectLoggingMetadata(ctx, log)

		ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
		if err != nil {
			log.WithError(err).Warn("owner account is invalid")
			return nil, status.Error(codes.Internal, "")
		}
		log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

		containerID, err := user.GetDataContainerIDFromProto(req.ContainerId)
		if err != nil {
			log.WithError(err).Warn("failure parsing data container id as uuid")
			return nil, status.Error(codes.Internal, "")
		}
		log = log.WithField("data_container", containerID.String())

		signature := req.Signature
		req.Signature = nil
		if err := s.auth.AuthorizeDataAccess(ctx, containerID, ownerAccount, req, signature); err != nil {
			return nil, err
		}

		userAgent, err := client.GetUserAgent(ctx)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid user-agent header value")
		}

		var tokenType push.TokenType

		switch userAgent.DeviceType {
		case client.DeviceTypeAndroid:
			if req.TokenType != pushpb.TokenType_FCM_ANDROID {
				return nil, status.Error(codes.InvalidArgument, "android client must specify an android token type")
			}

			tokenType = push.TokenTypeFcmAndroid
		case client.DeviceTypeIOS:
			if req.TokenType != pushpb.TokenType_FCM_APNS {
				return nil, status.Error(codes.InvalidArgument, "ios client must specify an apns token type")
			}

			tokenType = push.TokenTypeFcmApns
		default:
			return nil, status.Error(codes.InvalidArgument, "unsupported user-agent device type")
		}

		isValid, err := s.pushProvider.IsValidPushToken(ctx, req.PushToken)
		if err != nil {
			log.WithError(err).Warn("failure checking push token validity")
			return nil, status.Error(codes.Internal, "")
		} else if !isValid {
			return &pushpb.AddTokenResponse{
				Result: pushpb.AddTokenResponse_INVALID_PUSH_TOKEN,
			}, nil
		}

		record := &push.Record{
			DataContainerId: *containerID,

			PushToken: req.PushToken,
			TokenType: tokenType,
			IsValid:   true,

			CreatedAt: time.Now(),
		}
		if req.AppInstall != nil {
			record.AppInstallId = &req.AppInstall.Value
		}

		err = s.data.PutPushToken(ctx, record)
		if err != nil && err != push.ErrTokenExists {
			log.WithError(err).Warn("failure saving push token")
			return nil, status.Error(codes.Internal, "")
		}

		return &pushpb.AddTokenResponse{
			Result: pushpb.AddTokenResponse_OK,
		}, nil
	*/
	return nil, status.Error(codes.Unimplemented, "")
}
