package invite

import (
	"context"

	"github.com/sirupsen/logrus"

	invitepb "github.com/code-payments/code-protobuf-api/generated/go/invite/v2"

	"github.com/code-payments/code-server/pkg/phone"

	code_data "github.com/code-payments/code-server/pkg/code/data"
)

// The invite system has been removed, so any calls to this RPC results in success
// for legacy clients.
type inviteServer struct {
	log           *logrus.Entry
	data          code_data.Provider
	phoneVerifier phone.Verifier

	invitepb.UnimplementedInviteServer
}

func NewInviteServer(
	data code_data.Provider,
	phoneVerifier phone.Verifier,
) invitepb.InviteServer {
	return &inviteServer{
		log:           logrus.StandardLogger().WithField("type", "invite/v2/server"),
		data:          data,
		phoneVerifier: phoneVerifier,
	}
}

func (s *inviteServer) GetInviteCount(ctx context.Context, req *invitepb.GetInviteCountRequest) (*invitepb.GetInviteCountResponse, error) {
	return &invitepb.GetInviteCountResponse{
		Result:      invitepb.GetInviteCountResponse_OK,
		InviteCount: 0,
	}, nil
}

func (s *inviteServer) InvitePhoneNumber(ctx context.Context, req *invitepb.InvitePhoneNumberRequest) (*invitepb.InvitePhoneNumberResponse, error) {
	return &invitepb.InvitePhoneNumberResponse{
		Result: invitepb.InvitePhoneNumberResponse_OK,
	}, nil
}

func (s *inviteServer) GetInvitationStatus(ctx context.Context, req *invitepb.GetInvitationStatusRequest) (*invitepb.GetInvitationStatusResponse, error) {
	return &invitepb.GetInvitationStatusResponse{
		Result: invitepb.GetInvitationStatusResponse_OK,
		Status: invitepb.InvitationStatus_INVITED,
	}, nil
}
