package request

import (
	"context"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	micropaymentpb "github.com/code-payments/code-protobuf-api/generated/go/micropayment/v1"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/common"
)

// todo: Migrate to a generic HTTP -> gRPC with signed proto strategy

func (s *Server) createTrustlessRequest(ctx context.Context, request *trustlessRequest) (err error) {
	return s.createRequest(
		ctx,
		&messagingpb.SendMessageRequest{
			RendezvousKey: &messagingpb.RendezvousKey{
				Value: request.GetPublicRendezvousKey().PublicKey().ToBytes(),
			},
			Message: request.ToProtoMessage(),
			Signature: &commonpb.Signature{
				Value: request.clientSignature[:],
			},
		},
		request.webhookUrl,
	)
}

func (s *Server) createRequest(
	ctx context.Context,
	signedCreateRequest *messagingpb.SendMessageRequest,
	webhookUrl *string,
) error {
	messagingClient := messagingpb.NewMessagingClient(s.cc)
	microPaymentClient := micropaymentpb.NewMicroPaymentClient(s.cc)

	//
	// Part 1: Create the request.
	//

	// Note that retries are acceptable
	createResp, err := messagingClient.SendMessage(ctx, signedCreateRequest)
	if err != nil {
		return err
	} else if createResp.Result != messagingpb.SendMessageResponse_OK {
		return errors.Errorf("send message result %s", createResp.Result)
	}

	//
	// Part 2: Register webhook if URL was provided
	//

	if webhookUrl != nil {
		registerWebhookReq := &micropaymentpb.RegisterWebhookRequest{
			IntentId: &commonpb.IntentId{
				Value: signedCreateRequest.RendezvousKey.Value,
			},
			Url: *webhookUrl,
		}

		registerWebhookResp, err := microPaymentClient.RegisterWebhook(ctx, registerWebhookReq)
		if err != nil {
			return err
		}

		switch registerWebhookResp.Result {
		case micropaymentpb.RegisterWebhookResponse_OK, micropaymentpb.RegisterWebhookResponse_ALREADY_REGISTERED:
		default:
			return errors.Errorf("register webhook result %s", createResp.Result)
		}
	}

	return nil
}

type intentStatusResp struct {
	Status string
}

func (s *Server) getIntentStatus(ctx context.Context, intentId *common.Account) (*intentStatusResp, error) {
	microPaymentClient := micropaymentpb.NewMicroPaymentClient(s.cc)

	getStatusReq := &micropaymentpb.GetStatusRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.PublicKey().ToBytes(),
		},
	}

	getStatusResp, err := microPaymentClient.GetStatus(ctx, getStatusReq)
	if err != nil {
		return nil, err
	}

	res := intentStatusResp{
		Status: "UNKNOWN",
	}

	if getStatusResp.IntentSubmitted {
		res.Status = "SUBMITTED"
	} else if getStatusResp.Exists {
		res.Status = "PENDING"
	}

	return &res, nil
}

type getUserIdResp struct {
	User string
}

func (s *Server) getUserId(ctx context.Context, protoReq *userpb.GetLoginForThirdPartyAppRequest) (*getUserIdResp, error) {
	userIdentityClient := userpb.NewIdentityClient(s.cc)

	getLoginResp, err := userIdentityClient.GetLoginForThirdPartyApp(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	res := &getUserIdResp{}
	if getLoginResp.Result == userpb.GetLoginForThirdPartyAppResponse_OK {
		res.User = base58.Encode(getLoginResp.UserId.Value)
	}
	return res, nil
}
