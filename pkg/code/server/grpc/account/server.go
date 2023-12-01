package account

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountpb "github.com/code-payments/code-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/cache"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/kin"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

var (
	giftCardCacheByOwner = cache.NewCache(10_000)
)

type balanceMetadata struct {
	value  uint64
	source accountpb.TokenAccountInfo_BalanceSource
}

type server struct {
	log  *logrus.Entry
	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	accountpb.UnimplementedAccountServer
}

func NewAccountServer(data code_data.Provider) accountpb.AccountServer {
	return &server{
		log:  logrus.StandardLogger().WithField("type", "account/server"),
		data: data,
		auth: auth_util.NewRPCSignatureVerifier(data),
	}
}

func (s *server) IsCodeAccount(ctx context.Context, req *accountpb.IsCodeAccountRequest) (*accountpb.IsCodeAccountResponse, error) {
	log := s.log.WithField("method", "IsCodeAccount")
	log = client.InjectLoggingMetadata(ctx, log)

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

	state, err := common.GetOwnerManagementState(ctx, s.data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return nil, status.Error(codes.Internal, "")
	}

	var result accountpb.IsCodeAccountResponse_Result
	switch state {
	case common.OwnerManagementStateCodeAccount:
		result = accountpb.IsCodeAccountResponse_OK
	case common.OwnerManagementStateNotFound:
		result = accountpb.IsCodeAccountResponse_NOT_FOUND
	case common.OwnerManagementStateUnlocked:
		result = accountpb.IsCodeAccountResponse_UNLOCKED_TIMELOCK_ACCOUNT
	case common.OwnerManagementStateUnknown:
		log.Warn("unknown owner management state")
		return nil, status.Error(codes.Internal, "")
	}

	return &accountpb.IsCodeAccountResponse{
		Result: result,
	}, nil
}

func (s *server) GetTokenAccountInfos(ctx context.Context, req *accountpb.GetTokenAccountInfosRequest) (*accountpb.GetTokenAccountInfosResponse, error) {
	log := s.log.WithField("method", "GetTokenAccountInfos")
	log = client.InjectLoggingMetadata(ctx, log)

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

	cachedResp, ok := giftCardCacheByOwner.Retrieve(owner.PublicKey().ToBase58())
	if ok {
		return cachedResp.(*accountpb.GetTokenAccountInfosResponse), nil
	}

	// Fetch all account records
	recordsByType, err := common.GetLatestTokenAccountRecordsForOwner(ctx, s.data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting latest account records")
		return nil, status.Error(codes.Internal, "")
	}

	legacyPrimary2022Records, err := common.GetLegacyPrimary2022AccountRecordsIfNotMigrated(ctx, s.data, owner)
	if err != common.ErrNoPrivacyMigration2022 && err != nil {
		log.WithError(err).Warn("failure getting legacy 2022 account records")
		return nil, status.Error(codes.Internal, "")
	} else if err == nil {
		recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022] = []*common.AccountRecords{legacyPrimary2022Records}
	}

	// Trigger a deposit sync with the blockchain for the primary account, if it exists
	if primaryRecords, ok := recordsByType[commonpb.AccountType_PRIMARY]; ok {
		if !primaryRecords[0].General.RequiresDepositSync {
			primaryRecords[0].General.RequiresDepositSync = true
			err = s.data.UpdateAccountInfo(ctx, primaryRecords[0].General)
			if err != nil {
				log.WithError(err).WithField("token_account", primaryRecords[0].General.TokenAccount).Warn("failure marking primary account for deposit sync")
			}
		}
	}

	// Fetch balances
	balanceMetadataByTokenAccount := make(map[string]*balanceMetadata)

	// Pre-privacy accounts are not supported by the new batching stuff
	if _, ok := recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022]; ok {
		log := log.WithField("token_account", recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022][0].General.TokenAccount)

		tokenAccount, err := common.NewAccountFromPublicKeyString(recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022][0].General.TokenAccount)
		if err != nil {
			return nil, status.Error(codes.Internal, "")
		}

		quarks, err := balance.DefaultCalculation(ctx, s.data, tokenAccount)
		switch err {
		case nil:
			balanceMetadataByTokenAccount[tokenAccount.PublicKey().ToBase58()] = &balanceMetadata{
				value:  quarks,
				source: accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			}
		case balance.ErrNotManagedByCode:
			// Don't bother calculating a balance, the account isn't useable in Code
			balanceMetadataByTokenAccount[tokenAccount.PublicKey().ToBase58()] = &balanceMetadata{
				value:  0,
				source: accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN,
			}
		default:
			log.WithError(err).Warn("failure getting balance")
			return nil, status.Error(codes.Internal, "")
		}
	}

	// Privacy account balances can be fetched in a batched method
	var batchedAccountRecords []*common.AccountRecords
	for accountType, batchAccountRecords := range recordsByType {
		if accountType == commonpb.AccountType_LEGACY_PRIMARY_2022 {
			continue
		}

		for _, accountRecords := range batchAccountRecords {
			if common.IsManagedByCode(ctx, accountRecords.Timelock) {
				batchedAccountRecords = append(batchedAccountRecords, accountRecords)
			} else {
				// Don't bother calculating a balance, the account isn't useable in Code
				balanceMetadataByTokenAccount[accountRecords.General.TokenAccount] = &balanceMetadata{
					value:  0,
					source: accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN,
				}
			}
		}
	}
	balancesByTokenAccount, err := balance.DefaultBatchCalculationWithAccountRecords(ctx, s.data, batchedAccountRecords...)
	if err != nil {
		log.WithError(err).Warn("failure fetching batched balances")
		return nil, status.Error(codes.Internal, "")
	}
	for tokenAccount, quarks := range balancesByTokenAccount {
		balanceMetadataByTokenAccount[tokenAccount] = &balanceMetadata{
			value:  quarks,
			source: accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
		}
	}

	tokenAccountInfos := make(map[string]*accountpb.TokenAccountInfo)
	for _, batchRecords := range recordsByType {
		for _, records := range batchRecords {
			log := log.WithField("token_account", records.General.TokenAccount)

			proto, err := s.getProtoAccountInfo(ctx, records, balanceMetadataByTokenAccount[records.General.TokenAccount])
			if err != nil {
				log.WithError(err).Warn("failure getting proto account info")
				return nil, status.Error(codes.Internal, "")
			}

			tokenAccountInfos[records.General.TokenAccount] = proto
		}
	}

	if len(tokenAccountInfos) == 0 {
		return &accountpb.GetTokenAccountInfosResponse{
			Result: accountpb.GetTokenAccountInfosResponse_NOT_FOUND,
		}, nil
	}

	resp := &accountpb.GetTokenAccountInfosResponse{
		Result:            accountpb.GetTokenAccountInfosResponse_OK,
		TokenAccountInfos: tokenAccountInfos,
	}

	// Is this a gift card in a terminal state that we can cache?
	if _, ok := recordsByType[commonpb.AccountType_REMOTE_SEND_GIFT_CARD]; len(tokenAccountInfos) == 1 && ok {
		tokenAccountInfo := tokenAccountInfos[recordsByType[commonpb.AccountType_REMOTE_SEND_GIFT_CARD][0].General.TokenAccount]

		switch tokenAccountInfo.ClaimState {
		case accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED, accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED:
			giftCardCacheByOwner.Insert(owner.PublicKey().ToBase58(), resp, 1)
		}
	}

	return resp, nil
}

func (s *server) getProtoAccountInfo(ctx context.Context, records *common.AccountRecords, prefetchedBalanceMetadata *balanceMetadata) (*accountpb.TokenAccountInfo, error) {
	ownerAccount, err := common.NewAccountFromPublicKeyString(records.General.OwnerAccount)
	if err != nil {
		return nil, err
	}

	authorityAccount, err := common.NewAccountFromPublicKeyString(records.General.AuthorityAccount)
	if err != nil {
		return nil, err
	}

	tokenAccount, err := common.NewAccountFromPublicKeyString(records.General.TokenAccount)
	if err != nil {
		return nil, err
	}

	// todo: We don't yet handle the closing state
	var managementState accountpb.TokenAccountInfo_ManagementState
	switch records.Timelock.VaultState {
	case timelock_token_v1.StateUnknown:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNKNOWN
		if records.Timelock.Block == 0 {
			// We haven't observed the timelock account at all on the blockchain,
			// but we know it's guaranteed to be created by the intents system
			// in the locked state.
			managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED
		}
	case timelock_token_v1.StateUnlocked:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED
	case timelock_token_v1.StateWaitingForTimeout:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKING
	case timelock_token_v1.StateLocked:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED
	case timelock_token_v1.StateClosed:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED
	default:
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNKNOWN
	}

	// Should never happen and is a precautionary check. We can't manage timelock
	// accounts where we aren't the time authority.
	if records.Timelock.TimeAuthority != common.GetSubsidizer().PublicKey().ToBase58() {
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE
	}

	// Should never happen and is a precautionary check. We can't manage timelock
	// accounts where we aren't the close authority.
	if records.Timelock.CloseAuthority != common.GetSubsidizer().PublicKey().ToBase58() {
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE
	}

	blockchainState := accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST
	if records.Timelock.ExistsOnBlockchain() {
		blockchainState = accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS
	}

	mustRotate, err := s.shouldClientRotateAccount(ctx, records, prefetchedBalanceMetadata.value)
	if err != nil {
		return nil, err
	}

	// Claimed states only apply to gift card accounts
	var claimState accountpb.TokenAccountInfo_ClaimState
	if records.General.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		// Check for an explicit action to claim the gift card
		_, err = s.data.GetGiftCardClaimedAction(ctx, records.General.TokenAccount)
		switch err {
		case nil:
			claimState = accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED
		case action.ErrActionNotFound:
			if common.IsManagedByCode(ctx, records.Timelock) {
				claimState = accountpb.TokenAccountInfo_CLAIM_STATE_NOT_CLAIMED
			}
		default:
			return nil, err
		}

		// Otherwise, check whether it looks like the gift card was claimed.
		if prefetchedBalanceMetadata.source == accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE && prefetchedBalanceMetadata.value == 0 {
			claimState = accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED
		} else if records.Timelock.IsClosed() {
			claimState = accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED
		}

		// Finally, check the status of the auto-return action. This will correct
		// any false positive claim states not generated from an explicit action.
		autoReturnActionRecord, err := s.data.GetGiftCardAutoReturnAction(ctx, records.General.TokenAccount)
		switch err {
		case nil:
			if autoReturnActionRecord.State != action.StateUnknown {
				claimState = accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED
			}
		case action.ErrActionNotFound:
		default:
			return nil, err
		}

		// Unclaimed gift cards that are close to the auto-return window are
		// marked as expired in a consistent manner as SubmitIntent to avoid
		// race conditions with the auto-return.
		if claimState == accountpb.TokenAccountInfo_CLAIM_STATE_NOT_CLAIMED && time.Since(records.General.CreatedAt) > 24*time.Hour-15*time.Minute {
			claimState = accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED
		}

		// If the gift card account is claimed or expired, force the balance to zero.
		if claimState == accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED || claimState == accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED {
			prefetchedBalanceMetadata.source = accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE
			prefetchedBalanceMetadata.value = 0
		}
	}

	var originalExchangeData *transactionpb.ExchangeData
	if records.General.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		originalExchangeData, err = s.getOriginalGiftCardExchangeData(ctx, records)
		if err != nil {
			return nil, err
		}
	}

	return &accountpb.TokenAccountInfo{
		Address:              tokenAccount.ToProto(),
		Owner:                ownerAccount.ToProto(),
		Authority:            authorityAccount.ToProto(),
		AccountType:          records.General.AccountType,
		Index:                records.General.Index,
		BalanceSource:        prefetchedBalanceMetadata.source,
		Balance:              prefetchedBalanceMetadata.value,
		ManagementState:      managementState,
		BlockchainState:      blockchainState,
		MustRotate:           mustRotate,
		ClaimState:           claimState,
		OriginalExchangeData: originalExchangeData,
		Mint: &commonpb.SolanaAccountId{
			Value: kin.TokenMint,
		},
		MintDecimals:    kin.Decimals,
		MintDisplayName: "Kin",
	}, nil
}

func (s *server) shouldClientRotateAccount(ctx context.Context, records *common.AccountRecords, balance uint64) (bool, error) {
	// Only temp incoming accounts require server hints to rotate
	if records.General.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
		return false, nil
	}

	// Rotation should occur if the account has a balance
	return balance > 0, nil
}

func (s *server) getOriginalGiftCardExchangeData(ctx context.Context, records *common.AccountRecords) (*transactionpb.ExchangeData, error) {
	if records.General.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return nil, errors.New("invalid account type")
	}

	intentRecord, err := s.data.GetOriginalGiftCardIssuedIntent(ctx, records.General.TokenAccount)
	if err != nil {
		return nil, err
	}

	return &transactionpb.ExchangeData{
		Currency:     string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency),
		ExchangeRate: intentRecord.SendPrivatePaymentMetadata.ExchangeRate,
		NativeAmount: intentRecord.SendPrivatePaymentMetadata.NativeAmount,
		Quarks:       intentRecord.SendPrivatePaymentMetadata.Quantity,
	}, nil
}

func hideDust(quarks uint64) uint64 {
	return kin.ToQuarks(kin.FromQuarks(quarks))
}
