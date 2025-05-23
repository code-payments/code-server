package transaction_v2

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/onramp"
	"github.com/code-payments/code-server/pkg/code/limit"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

func (s *transactionServer) DeclareFiatOnrampPurchaseAttempt(ctx context.Context, req *transactionpb.DeclareFiatOnrampPurchaseAttemptRequest) (*transactionpb.DeclareFiatOnrampPurchaseAttemptResponse, error) {
	log := s.log.WithField("method", "DeclareFiatOnrampPurchaseAttempt")
	log = client.InjectLoggingMetadata(ctx, log)

	if common.CoreMintAccount.PublicKey().ToBase58() == common.UsdcMintAccount.PublicKey().ToBase58() {
		log.Warn("core mint account is usdc")
		return nil, status.Error(codes.Unavailable, "")
	}

	var deviceType client.DeviceType
	userAgent, err := client.GetUserAgent(ctx)
	if err == nil {
		deviceType = userAgent.DeviceType
	} else {
		deviceType = client.DeviceTypeUnknown
	}

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

	currency := currency_lib.Code(strings.ToLower(req.PurchaseAmount.Currency))
	amount := req.PurchaseAmount.NativeAmount

	nonce, err := uuid.FromBytes(req.Nonce.Value)
	if err != nil {
		log.WithError(err).Warn("nonce is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("nonce", nonce.String())

	// Validate the purchase amount makes sense within defined limits
	sendLimit, ok := limit.SendLimits[currency]
	if !ok {
		return &transactionpb.DeclareFiatOnrampPurchaseAttemptResponse{
			Result: transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_UNSUPPORTED_CURRENCY,
		}, nil
	} else if amount > sendLimit.PerTransaction {
		return &transactionpb.DeclareFiatOnrampPurchaseAttemptResponse{
			Result: transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_AMOUNT_EXCEEDS_MAXIMUM,
		}, nil
	}

	record := &onramp.Record{
		Owner:     owner.PublicKey().ToBase58(),
		Platform:  int(deviceType),
		Currency:  string(currency),
		Amount:    amount,
		Nonce:     nonce,
		CreatedAt: time.Now(),
	}
	err = s.data.PutFiatOnrampPurchase(ctx, record)
	if err != nil && err != onramp.ErrPurchaseAlreadyExists {
		log.WithError(err).Warn("failure creating fiat onramp purchase record")
		return nil, status.Error(codes.Internal, "")
	}

	recordBuyModulePurchaseInitiatedEvent(
		ctx,
		currency,
		amount,
		deviceType,
	)

	return &transactionpb.DeclareFiatOnrampPurchaseAttemptResponse{
		Result: transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_OK,
	}, nil
}
