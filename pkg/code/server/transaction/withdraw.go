package transaction_v2

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/token"
)

func (s *transactionServer) CanWithdrawToAccount(ctx context.Context, req *transactionpb.CanWithdrawToAccountRequest) (*transactionpb.CanWithdrawToAccountResponse, error) {
	log := s.log.WithField("method", "CanWithdrawToAccount")
	log = client.InjectLoggingMetadata(ctx, log)

	accountToCheck, err := common.NewAccountFromProto(req.Account)
	if err != nil {
		log.WithError(err).Warn("invalid account provided")
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	}
	log = log.WithField("account", accountToCheck.PublicKey().ToBase58())

	mintAccount, err := common.GetBackwardsCompatMint(req.Mint)
	if err != nil {
		log.WithError(err).Warn("invalid mint provided")
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	}
	log = log.WithField("mint", mintAccount.PublicKey().ToBase58())

	isOnCurve := accountToCheck.IsOnCurve()

	//
	// Part 1: Is this a Timelock vault? If so, only allow primary accounts.
	//

	if !isOnCurve {
		accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, accountToCheck.PublicKey().ToBase58())
		switch err {
		case nil:
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: accountInfoRecord.AccountType == commonpb.AccountType_PRIMARY,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			}, nil
		case account.ErrAccountInfoNotFound:
			// Nothing to do
		default:
			log.WithError(err).Warn("failure checking account info db")
			return nil, status.Error(codes.Internal, "")
		}
	}

	//
	// Part 2: Is this an opened mint token account? If so, allow it.
	//

	if !isOnCurve {
		_, err = s.data.GetBlockchainTokenAccountInfo(ctx, accountToCheck.PublicKey().ToBase58(), mintAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
		switch err {
		case nil:
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: true,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			}, nil
		case token.ErrAccountNotFound, solana.ErrNoAccountInfo, token.ErrInvalidTokenAccount:
			// Nothing to do
		default:
			log.WithError(err).Warn("failure checking against blockchain as a token account")
			return nil, status.Error(codes.Internal, "")
		}
	}

	//
	// Part 3: Is this an owner account with an opened mint ATA? If so, allow it.
	//         If not, indicate to the client to pay a fee for a create-on-send
	//         withdrawal.
	//

	var isVmDepositPda bool
	timelockRecord, err := s.data.GetTimelockByDepositPda(ctx, accountToCheck.PublicKey().ToBase58())
	switch err {
	case nil:
		isVmDepositPda = true
	case timelock.ErrTimelockNotFound:
	default:
		log.WithError(err).Warn("failure checking timelock db as a deposit pda account")
		return nil, status.Error(codes.Internal, "")
	}

	if !isOnCurve && !isVmDepositPda {
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	}

	if isVmDepositPda {
		accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, timelockRecord.VaultAddress)
		if err != nil {
			log.WithError(err).Warn("failure checking account info db")
			return nil, status.Error(codes.Internal, "")
		}

		if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY || accountInfoRecord.MintAccount != mintAccount.PublicKey().ToBase58() {
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: false,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
			}, nil
		}
	}

	ata, err := accountToCheck.ToAssociatedTokenAccount(mintAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting ata address")
		return nil, status.Error(codes.Internal, "")
	}

	_, err = s.data.GetBlockchainTokenAccountInfo(ctx, ata.PublicKey().ToBase58(), mintAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: true,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_OwnerAccount,
		}, nil
	case token.ErrAccountNotFound, solana.ErrNoAccountInfo:
		// ATA doesn't exist, and we won't be subsidizing it. Let the client know
		// they require a fee.
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: true,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_OwnerAccount,
			RequiresInitialization:    true,
			FeeAmount: &transactionpb.ExchangeDataWithoutRate{
				Currency:     string(currency_lib.USD),
				NativeAmount: s.conf.createOnSendWithdrawalUsdFee.Get(ctx),
			},
		}, nil
	case token.ErrInvalidTokenAccount:
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	default:
		log.WithError(err).Warn("failure checking against blockchain as an owner account")
		return nil, status.Error(codes.Internal, "")
	}
}
