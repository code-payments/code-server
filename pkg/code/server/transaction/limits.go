package transaction_v2

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/limit"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

func (s *transactionServer) GetLimits(ctx context.Context, req *transactionpb.GetLimitsRequest) (*transactionpb.GetLimitsResponse, error) {
	log := s.log.WithField("method", "GetLimits")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	sig := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, ownerAccount, req, sig); err != nil {
		return nil, err
	}

	sendLimits := make(map[string]*transactionpb.SendLimit)
	microPaymentLimits := make(map[string]*transactionpb.MicroPaymentLimit)
	buyModuleLimits := make(map[string]*transactionpb.BuyModuleLimit)
	for currency, limit := range limit.SendLimits {
		sendLimits[string(currency)] = &transactionpb.SendLimit{
			NextTransaction:   float32(limit.PerTransaction),
			MaxPerTransaction: float32(limit.PerTransaction),
			MaxPerDay:         float32(limit.Daily),
		}
		buyModuleLimits[string(currency)] = &transactionpb.BuyModuleLimit{
			MaxPerTransaction: float32(limit.PerTransaction),
			MinPerTransaction: float32(limit.PerTransaction / 10),
		}
	}
	for currency, limit := range limit.MicroPaymentLimits {
		microPaymentLimits[string(currency)] = &transactionpb.MicroPaymentLimit{
			MaxPerTransaction: float32(limit.Max),
			MinPerTransaction: float32(limit.Min),
		}
	}
	return &transactionpb.GetLimitsResponse{
		Result:                       transactionpb.GetLimitsResponse_OK,
		SendLimitsByCurrency:         sendLimits,
		MicroPaymentLimitsByCurrency: microPaymentLimits,
		BuyModuleLimitsByCurrency:    buyModuleLimits,
	}, nil
}
