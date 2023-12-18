package paymentrequest

import (
	"context"
	"crypto/ed25519"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	micropaymentpb "github.com/code-payments/code-protobuf-api/generated/go/micropayment/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	testWebhookEndpoint = "https://api.getcode.com/v1/testWebhook"
)

func (s *Server) createTestGetcodeTrustedPaymentRequest(ctx context.Context, paymentRequest *trustedPaymentRequest) (err error) {
	rendezvousKey := paymentRequest.GetPrivateRendezvousKey()

	sendMessageReq := &messagingpb.SendMessageRequest{
		RendezvousKey: &messagingpb.RendezvousKey{
			Value: rendezvousKey.PublicKey().ToBytes(),
		},
		Message: paymentRequest.ToProtoMessageWithVerifidDomain(pointer.String("app.getcode.com"), s.getcodeDomainVerifier),
	}

	// This is obviously the worst part about a trusted payment request. Server can
	// see the rendezvous private key, and sign a message with a different destination
	// to steal funds. We need to render the key useless by having all fields encoded
	// in the Kik code payload.
	sendMessageReq.Signature, err = signProtoMessage(sendMessageReq.Message, rendezvousKey)
	if err != nil {
		return err
	}

	return s.createPaymentRequest(
		ctx,
		sendMessageReq,
		true,
		pointer.String(testWebhookEndpoint), // todo: support webhook through API
	)
}

func (s *Server) createTrustlessPaymentRequest(ctx context.Context, paymentRequest *trustlessPaymentRequest) (err error) {
	return s.createPaymentRequest(
		ctx,
		&messagingpb.SendMessageRequest{
			RendezvousKey: &messagingpb.RendezvousKey{
				Value: paymentRequest.GetPublicRendezvousKey().PublicKey().ToBytes(),
			},
			Message: paymentRequest.ToProtoMessage(),
			Signature: &commonpb.Signature{
				Value: paymentRequest.clientSignature[:],
			},
		},
		false,
		paymentRequest.webhookUrl,
	)
}

func (s *Server) createPaymentRequest(
	ctx context.Context,
	signedCreateRequest *messagingpb.SendMessageRequest,
	isServerGenerated bool,
	webhookUrl *string,
) error {
	messagingClient := messagingpb.NewMessagingClient(s.cc)
	microPaymentClient := micropaymentpb.NewMicroPaymentClient(s.cc)

	//
	// Part 1: Check if the payment request already exists (when safe to do so)
	//

	// Only do this for server generated requests as an optimization. Otherwise,
	// we'll need to send the message every time to get robust validation that
	// this service cannot provide.
	if isServerGenerated {
		statusReq := &micropaymentpb.GetStatusRequest{
			IntentId: &commonpb.IntentId{
				Value: signedCreateRequest.RendezvousKey.Value,
			},
		}
		statusResp, err := microPaymentClient.GetStatus(ctx, statusReq)
		if err != nil {
			return err
		} else if statusResp.Exists {
			return nil
		}
	}

	//
	// Part 2: If not, create the payment request.
	//

	// Note that retries are acceptable
	createResp, err := messagingClient.SendMessage(ctx, signedCreateRequest)
	if err != nil {
		return err
	} else if createResp.Result != messagingpb.SendMessageResponse_OK {
		return errors.Errorf("send message result %s", createResp.Result)
	}

	//
	// Part 3: Register webhook if URL was provided
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

func (s *Server) getIntentStatus(ctx context.Context, intentId *common.Account) (string, error) {
	microPaymentClient := micropaymentpb.NewMicroPaymentClient(s.cc)

	getStatusReq := &micropaymentpb.GetStatusRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.PublicKey().ToBytes(),
		},
	}

	getStatusResp, err := microPaymentClient.GetStatus(ctx, getStatusReq)
	if err != nil {
		return "", err
	}

	if getStatusResp.IntentSubmitted {
		return "SUBMITTED", nil
	} else if getStatusResp.Exists {
		return "PENDING", nil
	}
	return "UNKNOWN", nil
}

func signProtoMessage(msg proto.Message, account *common.Account) (*commonpb.Signature, error) {
	bytesToSign, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return &commonpb.Signature{
		Value: ed25519.Sign(account.PrivateKey().ToBytes(), bytesToSign),
	}, nil
}
