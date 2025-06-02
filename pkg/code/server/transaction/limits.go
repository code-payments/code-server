package transaction_v2

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
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

	multiRateRecord, err := s.data.GetAllExchangeRates(ctx, currency_util.GetLatestExchangeRateTime())
	if err != nil {
		log.WithError(err).Warn("failure getting current exchange rates")
		return nil, status.Error(codes.Internal, "")
	}

	usdRate, ok := multiRateRecord.Rates[string(currency_lib.USD)]
	if !ok {
		log.WithError(err).Warn("usd rate is missing")
		return nil, status.Error(codes.Internal, "")
	}

	_, consumedUsdForPayments, err := s.data.GetTransactedAmountForAntiMoneyLaundering(ctx, ownerAccount.PublicKey().ToBase58(), req.ConsumedSince.AsTime())
	if err != nil {
		log.WithError(err).Warn("failure calculating consumed usd payment value")
		return nil, status.Error(codes.Internal, "")
	}

	// Calculate send limits
	sendLimits := make(map[string]*transactionpb.SendLimit)
	for currency, sendLimit := range currency_util.SendLimits {
		otherRate, ok := multiRateRecord.Rates[string(currency)]
		if !ok {
			log.Debugf("%s rate is missing", currency)
			continue
		}

		// How much have we consumed in the other currency?
		consumedInOtherCurrency := consumedUsdForPayments * otherRate / usdRate

		// How much of the daily limit is remaining?
		remainingDaily := sendLimit.Daily - consumedInOtherCurrency

		// The per-transaction limit applies up until our remaining daily limit is below it.
		remainingNextTransaction := sendLimit.PerTransaction
		if remainingDaily < remainingNextTransaction {
			remainingNextTransaction = remainingDaily
		}

		// Avoid negative limits, possibly caused by fluctuating exchange rates
		if remainingNextTransaction < 0 {
			remainingNextTransaction = 0
		}

		sendLimits[string(currency)] = &transactionpb.SendLimit{
			NextTransaction:   float32(remainingNextTransaction),
			MaxPerTransaction: float32(sendLimit.PerTransaction),
			MaxPerDay:         float32(sendLimit.Daily),
		}
	}

	usdSendLimits := sendLimits[string(currency_lib.USD)]

	// Inject a core mint limit based on the remaining USD amount and rate
	sendLimits[string(common.CoreMintSymbol)] = &transactionpb.SendLimit{
		NextTransaction:   usdSendLimits.NextTransaction / float32(usdRate),
		MaxPerTransaction: usdSendLimits.MaxPerTransaction / float32(usdRate),
		MaxPerDay:         usdSendLimits.MaxPerDay / float32(usdRate),
	}

	return &transactionpb.GetLimitsResponse{
		Result:               transactionpb.GetLimitsResponse_OK,
		SendLimitsByCurrency: sendLimits,
		UsdTransacted:        consumedUsdForPayments,
	}, nil
}
