package device

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	devicepb "github.com/code-payments/code-protobuf-api/generated/go/device/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/login"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

type server struct {
	log  *logrus.Entry
	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	devicepb.UnimplementedDeviceServer
}

func NewDeviceServer(data code_data.Provider, auth *auth_util.RPCSignatureVerifier) devicepb.DeviceServer {
	return &server{
		log:  logrus.StandardLogger().WithField("type", "device/server"),
		data: data,
		auth: auth,
	}
}

func (s *server) RegisterLoggedInAccounts(ctx context.Context, req *devicepb.RegisterLoggedInAccountsRequest) (*devicepb.RegisterLoggedInAccountsResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":      "RegisterLoggedInAccounts",
		"app_install": req.AppInstall.Value,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	if len(req.Owners) != len(req.Signatures) {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	signatures := req.Signatures
	req.Signatures = nil

	var validOwners []string
	var invalidOwners []*commonpb.SolanaAccountId

	for i, protoOwner := range req.Owners {
		owner, err := common.NewAccountFromProto(protoOwner)
		if err != nil {
			log.WithError(err).Warn("invalid owner account")
			return nil, status.Error(codes.Internal, "")
		}

		if err := s.auth.Authenticate(ctx, owner, req, signatures[i]); err != nil {
			return nil, err
		}

		// todo: invalid owner detection

		validOwners = append(validOwners, owner.PublicKey().ToBase58())
	}

	if len(invalidOwners) > 0 {
		return &devicepb.RegisterLoggedInAccountsResponse{
			Result:        devicepb.RegisterLoggedInAccountsResponse_INVALID_OWNER,
			InvalidOwners: invalidOwners,
		}, nil
	}

	loginRecord := &login.MultiRecord{
		AppInstallId: req.AppInstall.Value,
		Owners:       validOwners,
	}
	err := s.data.SaveLogins(ctx, loginRecord)
	if err != nil {
		log.WithError(err).Warn("failure updating login records")
		return nil, status.Error(codes.Internal, "")
	}

	return &devicepb.RegisterLoggedInAccountsResponse{
		Result: devicepb.RegisterLoggedInAccountsResponse_OK,
	}, nil
}

func (s *server) GetLoggedInAccounts(ctx context.Context, req *devicepb.GetLoggedInAccountsRequest) (*devicepb.GetLoggedInAccountsResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":      "GetLoggedInAccounts",
		"app_install": req.AppInstall.Value,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	var protoOwners []*commonpb.SolanaAccountId

	loginRecord, err := s.data.GetLoginsByAppInstall(ctx, req.AppInstall.Value)
	switch err {
	case nil:
		for _, owner := range loginRecord.Owners {
			parsed, err := common.NewAccountFromPublicKeyString(owner)
			if err != nil {
				log.WithError(err).Warn("invalid owner account")
				return nil, status.Error(codes.Internal, "")
			}

			protoOwners = append(protoOwners, parsed.ToProto())
		}
	case login.ErrLoginNotFound:
	default:
		log.WithError(err).Warn("failure getting login records")
		return nil, status.Error(codes.Internal, "")
	}

	if len(protoOwners) > 1 {
		log.Warn("unexpectedly have more than 1 owner logged into same app install")
		return nil, status.Error(codes.Internal, "")
	}

	return &devicepb.GetLoggedInAccountsResponse{
		Result: devicepb.GetLoggedInAccountsResponse_OK,
		Owners: protoOwners,
	}, nil
}
