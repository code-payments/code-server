package micropayment

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	micropaymentpb "github.com/code-payments/code-protobuf-api/generated/go/micropayment/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/paywall"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/limit"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/retry"
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

	_, err := s.data.GetPaymentRequest(ctx, intentId)
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return resp, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting payment request record")
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

	_, err = s.data.GetPaymentRequest(ctx, intentId)
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return &micropaymentpb.RegisterWebhookResponse{
			Result: micropaymentpb.RegisterWebhookResponse_PAYMENT_REQUEST_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure checking payment request status")
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

func (s *microPaymentServer) Codify(ctx context.Context, req *micropaymentpb.CodifyRequest) (*micropaymentpb.CodifyResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "Codify",
	})
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.OwnerAccount)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	destination, err := common.NewAccountFromProto(req.PrimaryAccount)
	if err != nil {
		log.WithError(err).Warn("invalid destination account")
		return nil, status.Error(codes.Internal, "")
	}

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	err = netutil.ValidateHttpUrl(req.Url, false, true)
	if err != nil {
		log.WithField("url", req.Url).WithError(err).Info("url failed validation")
		return &micropaymentpb.CodifyResponse{
			Result: micropaymentpb.CodifyResponse_INVALID_URL,
		}, nil
	}

	primaryAccountInfoRecord, err := s.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, owner.PublicKey().ToBase58(), commonpb.AccountType_PRIMARY)
	if err == account.ErrAccountInfoNotFound {
		return &micropaymentpb.CodifyResponse{
			Result: micropaymentpb.CodifyResponse_INVALID_ACCOUNT,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting primary account info record")
		return nil, status.Error(codes.Internal, "")
	}

	if primaryAccountInfoRecord.TokenAccount != destination.PublicKey().ToBase58() {
		return &micropaymentpb.CodifyResponse{
			Result: micropaymentpb.CodifyResponse_INVALID_ACCOUNT,
		}, nil
	}

	limits, ok := limit.MicroPaymentLimits[currency_lib.Code(req.Currency)]
	if !ok {
		return &micropaymentpb.CodifyResponse{
			Result: micropaymentpb.CodifyResponse_UNSUPPORTED_CURRENCY,
		}, nil
	} else if req.NativeAmount > limits.Max || req.NativeAmount < limits.Min {
		return &micropaymentpb.CodifyResponse{
			Result: micropaymentpb.CodifyResponse_NATIVE_AMOUNT_EXCEEDS_LIMIT,
		}, nil
	}

	var shortPath string
	_, err = retry.Retry( // In the unlikely event getRandomShortPath has a collision
		func() error {
			shortPath = getRandomShortPath()

			return s.data.CreatePaywall(ctx, &paywall.Record{
				OwnerAccount:            owner.PublicKey().ToBase58(),
				DestinationTokenAccount: destination.PublicKey().ToBase58(),

				ExchangeCurrency: currency_lib.Code(req.Currency),
				NativeAmount:     req.NativeAmount,
				RedirectUrl:      req.Url,
				ShortPath:        shortPath,

				Signature: base58.Encode(signature.Value),

				CreatedAt: time.Now(),
			})
		},
		retry.RetriableErrors(paywall.ErrPaywallExists),
		retry.Limit(3),
	)
	if err != nil {
		log.WithError(err).Warn("failure creating paywall record")
		return nil, status.Error(codes.Internal, "")
	}

	return &micropaymentpb.CodifyResponse{
		Result:      micropaymentpb.CodifyResponse_OK,
		CodifiedUrl: codifiedContentUrlBase + shortPath,
	}, nil
}

func (s *microPaymentServer) GetPathMetadata(ctx context.Context, req *micropaymentpb.GetPathMetadataRequest) (*micropaymentpb.GetPathMetadataResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "GetPathMetadata",
		"path":   req.Path,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	paywallRecord, err := s.data.GetPaywallByShortPath(ctx, req.Path)
	if err == paywall.ErrPaywallNotFound {
		return &micropaymentpb.GetPathMetadataResponse{
			Result: micropaymentpb.GetPathMetadataResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting paywall record")
		return nil, status.Error(codes.Internal, "")
	}

	destination, err := common.NewAccountFromPublicKeyString(paywallRecord.DestinationTokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid destination account")
		return nil, status.Error(codes.Internal, "")
	}

	// Hard-coded stub implementation to enable a PoC
	return &micropaymentpb.GetPathMetadataResponse{
		Result:       micropaymentpb.GetPathMetadataResponse_OK,
		Destination:  destination.ToProto(),
		Currency:     string(paywallRecord.ExchangeCurrency),
		NativeAmount: paywallRecord.NativeAmount,
		RedirctUrl:   paywallRecord.RedirectUrl,
	}, nil
}

// Something stupidly simple to start
func getRandomShortPath() string {
	rawValue := make([]byte, 4)
	binary.LittleEndian.PutUint32(rawValue, rand.Uint32())
	path := base64.StdEncoding.EncodeToString(rawValue)
	path = strings.Replace(path, "/", "", -1)
	path = strings.Replace(path, "=", "", -1)
	path = strings.Replace(path, "+", "", -1)
	return path
}
