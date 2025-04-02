package micropayment

import (
	"context"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	micropaymentpb "github.com/code-payments/code-protobuf-api/generated/go/micropayment/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/netutil"
)

const (
	codifiedContentUrlBase = "getcode.com/m/"
)

type microPaymentServer struct {
	log *logrus.Entry

	data code_data.Provider

	auth *auth_util.RPCSignatureVerifier

	micropaymentpb.UnimplementedMicroPaymentServer
}

func NewMicroPaymentServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
) micropaymentpb.MicroPaymentServer {
	return &microPaymentServer{
		log:  logrus.StandardLogger().WithField("type", "micropayment/v1/server"),
		data: data,
		auth: auth,
	}
}

func (s *microPaymentServer) GetStatus(ctx context.Context, req *micropaymentpb.GetStatusRequest) (*micropaymentpb.GetStatusResponse, error) {
	log := s.log.WithField("method", "GetStatus")
	log = client.InjectLoggingMetadata(ctx, log)

	intentId := base58.Encode(req.IntentId.Value)
	log = log.WithField("intent", intentId)

	resp := &micropaymentpb.GetStatusResponse{
		Exists:          false,
		CodeScanned:     false,
		IntentSubmitted: false,
	}

	_, err := s.data.GetRequest(ctx, intentId)
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return resp, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting request record")
		return nil, status.Error(codes.Internal, "")
	}
	resp.Exists = true

	messageRecords, err := s.data.GetMessages(ctx, intentId)
	if err != nil {
		log.WithError(err).Warn("failure getting message records")
		return nil, status.Error(codes.Internal, "")
	}

	for _, messageRecord := range messageRecords {
		var message messagingpb.Message
		if err := proto.Unmarshal(messageRecord.Message, &message); err != nil {
			log.WithError(err).Warn("failure unmarshalling message bytes")
			continue
		}

		switch message.Kind.(type) {
		case *messagingpb.Message_CodeScanned:
			resp.CodeScanned = true
		}

		if resp.CodeScanned {
			break
		}
	}

	intentRecord, err := s.data.GetIntent(ctx, intentId)
	switch err {
	case nil:
		resp.IntentSubmitted = intentRecord.State != intent.StateRevoked
	case intent.ErrIntentNotFound:
	default:
		log.WithError(err).Warn("failure getting intent record")
		return nil, status.Error(codes.Internal, "")
	}

	return resp, nil
}

func (s *microPaymentServer) RegisterWebhook(ctx context.Context, req *micropaymentpb.RegisterWebhookRequest) (*micropaymentpb.RegisterWebhookResponse, error) {
	log := s.log.WithField("method", "RegisterWebhook")
	log = client.InjectLoggingMetadata(ctx, log)

	intentId := base58.Encode(req.IntentId.Value)
	log = log.WithField("intent", intentId)

	err := netutil.ValidateHttpUrl(req.Url, false, false)
	if err != nil {
		log.WithField("url", req.Url).WithError(err).Info("url failed validation")
		return &micropaymentpb.RegisterWebhookResponse{
			Result: micropaymentpb.RegisterWebhookResponse_INVALID_URL,
		}, nil
	}

	// todo: distributed lock on intent id

	_, err = s.data.GetIntent(ctx, intentId)
	if err == nil {
		return &micropaymentpb.RegisterWebhookResponse{
			Result: micropaymentpb.RegisterWebhookResponse_INTENT_EXISTS,
		}, nil
	} else if err != intent.ErrIntentNotFound {
		log.WithError(err).Warn("failure checking intent status")
		return nil, status.Error(codes.Internal, "")
	}

	_, err = s.data.GetRequest(ctx, intentId)
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return &micropaymentpb.RegisterWebhookResponse{
			Result: micropaymentpb.RegisterWebhookResponse_REQUEST_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure checking request status")
		return nil, status.Error(codes.Internal, "")
	}

	record := &webhook.Record{
		WebhookId: intentId,
		Url:       req.Url,
		Type:      webhook.TypeIntentSubmitted,

		Attempts: 0,
		State:    webhook.StateUnknown,

		CreatedAt:     time.Now(),
		NextAttemptAt: nil,
	}
	err = s.data.CreateWebhook(ctx, record)
	if err == webhook.ErrAlreadyExists {
		return &micropaymentpb.RegisterWebhookResponse{
			Result: micropaymentpb.RegisterWebhookResponse_ALREADY_REGISTERED,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure creating webhook record")
		return nil, status.Error(codes.Internal, "")
	}

	return &micropaymentpb.RegisterWebhookResponse{
		Result: micropaymentpb.RegisterWebhookResponse_OK,
	}, nil
}
