package badge

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	badgepb "github.com/code-payments/code-protobuf-api/generated/go/badge/v1"

	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/push"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_util "github.com/code-payments/code-server/pkg/code/push"
)

type server struct {
	log          *logrus.Entry
	data         code_data.Provider
	auth         *auth_util.RPCSignatureVerifier
	pushProvider push.Provider

	badgepb.UnimplementedBadgeServer
}

func NewBadgeServer(data code_data.Provider, pushProvider push.Provider, auth *auth_util.RPCSignatureVerifier) badgepb.BadgeServer {
	return &server{
		log:          logrus.StandardLogger().WithField("type", "badge/server"),
		data:         data,
		auth:         auth,
		pushProvider: pushProvider,
	}
}

func (s *server) ResetBadgeCount(ctx context.Context, req *badgepb.ResetBadgeCountRequest) (*badgepb.ResetBadgeCountResponse, error) {
	log := s.log.WithField("method", "ResetBadgeCount")
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

	err = s.data.ResetBadgeCount(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure resetting db badge count")
		return nil, status.Error(codes.Internal, "")
	}

	err = push_util.UpdateBadgeCount(ctx, s.data, s.pushProvider, owner)
	if err != nil {
		log.WithError(err).Warn("failure updating badge count on device")
		return nil, status.Error(codes.Internal, "")
	}

	return &badgepb.ResetBadgeCountResponse{
		Result: badgepb.ResetBadgeCountResponse_OK,
	}, nil
}
