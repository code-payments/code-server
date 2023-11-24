package transaction_v2

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/code/limit"
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

	zeroSendLimits := make(map[string]*transactionpb.RemainingSendLimit)
	zeroMicroPaymentLimits := make(map[string]*transactionpb.MicroPaymentLimit)
	for currency := range limit.SendLimits {
		zeroSendLimits[string(currency)] = &transactionpb.RemainingSendLimit{
			NextTransaction: 0,
		}
		zeroMicroPaymentLimits[string(currency)] = &transactionpb.MicroPaymentLimit{
			MaxPerTransaction: 0,
			MinPerTransaction: 0,
		}
	}
	zeroSendLimits[string(currency_lib.KIN)] = &transactionpb.RemainingSendLimit{
		NextTransaction: 0,
	}
	zeroMicroPaymentLimits[string(currency_lib.KIN)] = &transactionpb.MicroPaymentLimit{
		MaxPerTransaction: 0,
		MinPerTransaction: 0,
	}
	zeroResp := &transactionpb.GetLimitsResponse{
		Result:                        transactionpb.GetLimitsResponse_OK,
		RemainingSendLimitsByCurrency: zeroSendLimits,
		DepositLimit: &transactionpb.DepositLimit{
			MaxQuarks: 0,
		},
		MicroPaymentLimitsByCurrency: zeroMicroPaymentLimits,
	}

	verificationRecord, err := s.data.GetLatestPhoneVerificationForAccount(ctx, ownerAccount.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// We've detected an account that's not verified, so set all limits to 0
		return zeroResp, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting phone verification record")
		return nil, status.Error(codes.Internal, "")
	}

	_, consumedUsdForPayments, err := s.data.GetTransactedAmountForAntiMoneyLaundering(ctx, verificationRecord.PhoneNumber, req.ConsumedSince.AsTime())
	if err != nil {
		log.WithError(err).Warn("failure calculating consumed usd payment value")
		return nil, status.Error(codes.Internal, "")
	}

	_, consumedUsdForDeposits, err := s.data.GetDepositedAmountForAntiMoneyLaundering(ctx, verificationRecord.PhoneNumber, req.ConsumedSince.AsTime())
	if err != nil {
		log.WithError(err).Warn("failure calculating consumed usd payment value")
		return nil, status.Error(codes.Internal, "")
	}

	privateBalance, err := balance.GetPrivateBalance(ctx, s.data, ownerAccount)
	if err == balance.ErrNotManagedByCode {
		// We've detected an account that's not managed by code, so set all limits to 0
		return zeroResp, nil
	} else if err != nil {
		log.WithError(err).Warn("failure calculating owner account private balance")
		return nil, status.Error(codes.Internal, "")
	}

	multiRateRecord, err := s.data.GetAllExchangeRates(ctx, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		log.WithError(err).Warn("failure getting current exchange rates")
		return nil, status.Error(codes.Internal, "")
	}

	usdRate, ok := multiRateRecord.Rates[string(currency_lib.USD)]
	if !ok {
		log.WithError(err).Warn("usd rate is missing")
		return nil, status.Error(codes.Internal, "")
	}

	//
	// Part 1: Calculate send limits
	//

	remainingSendLimits := make(map[string]*transactionpb.RemainingSendLimit)
	for currency, sendLimit := range limit.SendLimits {
		otherRate, ok := multiRateRecord.Rates[string(currency)]
		if !ok {
			log.WithError(err).Warnf("%s rate is missing", currency)
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

		remainingSendLimits[string(currency)] = &transactionpb.RemainingSendLimit{
			NextTransaction: float32(remainingNextTransaction),
		}
	}

	usdSendLimits := remainingSendLimits[string(currency_lib.USD)]

	// Inject a Kin limit based on the remaining USD amount and rate
	remainingSendLimits[string(currency_lib.KIN)] = &transactionpb.RemainingSendLimit{
		NextTransaction: usdSendLimits.NextTransaction / float32(usdRate),
	}

	//
	// Part 2: Calculate deposit limits
	//

	usdForNextDeposit := limit.MaxPerDepositUsdAmount

	// Does the user already have sufficient balance in their organizer? If so,
	// then the limit is completely nullified.
	maxPerDepositQuarkAmount := kin.ToQuarks(uint64(limit.MaxPerDepositUsdAmount / usdRate))
	if privateBalance >= maxPerDepositQuarkAmount {
		usdForNextDeposit = 0
	}

	// How much of the daily limit is remaining?
	remainingUsdForDeposits := limit.MaxDailyDepositUsdAmount - consumedUsdForDeposits

	// The per-transaction limit applies up until our remaining daily limit is below it.
	if remainingUsdForDeposits < usdForNextDeposit {
		usdForNextDeposit = remainingUsdForDeposits
	}

	// Avoid negative limits, possibly caused by fluctuating exchange rates
	if usdForNextDeposit < 0 {
		usdForNextDeposit = 0
	}

	//
	// Part 3: Calculate micro payment limits
	//

	convertedMicroPaymentLimits := make(map[string]*transactionpb.MicroPaymentLimit)
	for currency, limits := range limit.MicroPaymentLimits {
		convertedMicroPaymentLimits[string(currency)] = &transactionpb.MicroPaymentLimit{
			MaxPerTransaction: float32(limits.Max),
			MinPerTransaction: float32(limits.Min),
		}
	}

	return &transactionpb.GetLimitsResponse{
		Result:                        transactionpb.GetLimitsResponse_OK,
		RemainingSendLimitsByCurrency: remainingSendLimits,
		DepositLimit: &transactionpb.DepositLimit{
			MaxQuarks: kin.ToQuarks(uint64(usdForNextDeposit / usdRate)),
		},
		MicroPaymentLimitsByCurrency: convertedMicroPaymentLimits,
	}, nil
}
