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

	zeroSendLimits := make(map[string]*transactionpb.SendLimit)
	zeroMicroPaymentLimits := make(map[string]*transactionpb.MicroPaymentLimit)
	zeroBuyModuleLimits := make(map[string]*transactionpb.BuyModuleLimit)
	for currency := range limit.SendLimits {
		zeroSendLimits[string(currency)] = &transactionpb.SendLimit{
			NextTransaction:   0,
			MaxPerTransaction: 0,
			MaxPerDay:         0,
		}
		zeroBuyModuleLimits[string(currency)] = &transactionpb.BuyModuleLimit{
			MaxPerTransaction: 0,
			MinPerTransaction: 0,
		}
	}
	for currency := range limit.MicroPaymentLimits {
		zeroMicroPaymentLimits[string(currency)] = &transactionpb.MicroPaymentLimit{
			MaxPerTransaction: 0,
			MinPerTransaction: 0,
		}
	}
	zeroResp := &transactionpb.GetLimitsResponse{
		Result:                       transactionpb.GetLimitsResponse_OK,
		SendLimitsByCurrency:         zeroSendLimits,
		MicroPaymentLimitsByCurrency: zeroMicroPaymentLimits,
		BuyModuleLimitsByCurrency:    zeroBuyModuleLimits,
		DepositLimit: &transactionpb.DepositLimit{
			MaxQuarks: 0,
		},
	}

	// todo: We need to calculate limits based on account, or another identity system
	//       now that phone numbers is no longer a requirements.
	return zeroResp, nil
}
