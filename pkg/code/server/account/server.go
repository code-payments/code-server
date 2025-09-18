package account

import (
	"context"
	"errors"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountpb "github.com/code-payments/code-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/cache"
	async_account "github.com/code-payments/code-server/pkg/code/async/account"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/grpc/client"
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

	ownerMetadata, err := common.GetOwnerMetadata(ctx, s.data, owner)
	if err == common.ErrOwnerNotFound {
		return &accountpb.IsCodeAccountResponse{
			Result: accountpb.IsCodeAccountResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return nil, status.Error(codes.Internal, "")
	}

	if ownerMetadata.Type != common.OwnerTypeUser12Words {
		return &accountpb.IsCodeAccountResponse{
			Result: accountpb.IsCodeAccountResponse_NOT_FOUND,
		}, nil
	}

	var result accountpb.IsCodeAccountResponse_Result
	switch ownerMetadata.State {
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

	ownerSignature := req.Signature
	req.Signature = nil

	var requestingOwner *common.Account
	var requestingOwnerSignature *commonpb.Signature
	if req.RequestingOwner != nil {
		requestingOwner, err = common.NewAccountFromProto(req.RequestingOwner)
		if err != nil {
			log.WithError(err).Warn("invalid requesting owner account")
			return nil, status.Error(codes.Internal, "")
		}
		log = log.WithField("requesting_owner_account", requestingOwner.PublicKey().ToBase58())

		requestingOwnerSignature = req.RequestingOwnerSignature
		req.RequestingOwnerSignature = nil
	}

	if err := s.auth.Authenticate(ctx, owner, req, ownerSignature); err != nil {
		return nil, err
	}
	if requestingOwner != nil {
		if err := s.auth.Authenticate(ctx, requestingOwner, req, requestingOwnerSignature); err != nil {
			return nil, err
		}
	}

	cachedResp, ok := giftCardCacheByOwner.Retrieve(owner.PublicKey().ToBase58())
	if ok {
		cachedResp := cachedResp.(*accountpb.GetTokenAccountInfosResponse)

		s.updateCachedResponse(cachedResp)

		resp, err := s.addRequestingOwnerMetadata(ctx, cachedResp, requestingOwner)
		if err != nil {
			log.WithError(err).Warn("failure adding requesting owner metadata")
			return nil, status.Error(codes.Internal, "")
		}
		return resp, nil
	}

	// Fetch all account records
	allRecordsByMintAndType, err := common.GetLatestTokenAccountRecordsForOwner(ctx, s.data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting latest account records")
		return nil, status.Error(codes.Internal, "")
	}

	var hasGiftCardAccount bool
	var allRecords []*common.AccountRecords
	for _, recordsByType := range allRecordsByMintAndType {
		for _, batchRecords := range recordsByType {
			for _, records := range batchRecords {
				if records.General.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
					hasGiftCardAccount = true
				}
				allRecords = append(allRecords, records)
			}
		}
	}

	// Filter account records based on client request
	//
	// todo: this needs tests
	var filteredRecords []*common.AccountRecords
	switch req.Filter.(type) {
	case *accountpb.GetTokenAccountInfosRequest_FilterByTokenAddress,
		*accountpb.GetTokenAccountInfosRequest_FilterByAccountType,
		*accountpb.GetTokenAccountInfosRequest_FilterByMintAddress:
		if hasGiftCardAccount {
			return nil, status.Error(codes.InvalidArgument, "filter must be nil for gift card owner accounts")
		}
	}
	switch typed := req.Filter.(type) {
	case *accountpb.GetTokenAccountInfosRequest_FilterByTokenAddress:
		filterAccount, err := common.NewAccountFromProto(typed.FilterByTokenAddress)
		if err != nil {
			log.WithError(err).Warn("invalid token address filter")
			return nil, status.Error(codes.Internal, "")
		}
		for _, records := range allRecords {
			if records.General.TokenAccount == filterAccount.PublicKey().ToBase58() {
				filteredRecords = append(filteredRecords, records)
				break
			}
		}
	case *accountpb.GetTokenAccountInfosRequest_FilterByAccountType:
		for _, records := range allRecords {
			if records.General.AccountType == typed.FilterByAccountType {
				filteredRecords = append(filteredRecords, records)
			}
		}
	case *accountpb.GetTokenAccountInfosRequest_FilterByMintAddress:
		filterAccount, err := common.NewAccountFromProto(typed.FilterByMintAddress)
		if err != nil {
			log.WithError(err).Warn("invalid mint address filter")
			return nil, status.Error(codes.Internal, "")
		}
		for _, records := range allRecords {
			if records.General.MintAccount == filterAccount.PublicKey().ToBase58() {
				filteredRecords = append(filteredRecords, records)
			}
		}
	default:
		filteredRecords = allRecords
	}

	var nextPoolIndex uint64
	latestPoolAccountInfoRecordByMint, err := s.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, owner.PublicKey().ToBase58(), commonpb.AccountType_POOL)
	switch err {
	case nil:
		// Pool accounts are only supported with the core mint
		latestCoreMintPoolAccountInfoRecord, ok := latestPoolAccountInfoRecordByMint[common.CoreMintAccount.PublicKey().ToBase58()]
		if ok {
			nextPoolIndex = latestCoreMintPoolAccountInfoRecord.Index + 1
		}
	case account.ErrAccountInfoNotFound:
		nextPoolIndex = 0
	default:
		log.WithError(err).Warn("failure getting latest pool account record")
		return nil, status.Error(codes.Internal, "")
	}

	// Trigger a deposit sync with the blockchain for the primary account, if it exists
	for _, records := range filteredRecords {
		if records.General.AccountType == commonpb.AccountType_PRIMARY && !records.General.RequiresDepositSync {
			records.General.RequiresDepositSync = true
			err = s.data.UpdateAccountInfo(ctx, records.General)
			if err != nil {
				log.WithError(err).WithField("token_account", records.General.TokenAccount).Warn("failure marking primary account for deposit sync")
			}
		}
	}

	// Fetch balances
	balanceMetadataByTokenAccount, err := s.fetchBalances(ctx, filteredRecords)
	if err != nil {
		log.WithError(err).Warn("failure fetching balances")
		return nil, status.Error(codes.Internal, "")
	}

	// Construct token account info
	tokenAccountInfos := make(map[string]*accountpb.TokenAccountInfo)
	for _, records := range filteredRecords {
		log := log.WithField("token_account", records.General.TokenAccount)

		proto, err := s.getProtoAccountInfo(ctx, records, balanceMetadataByTokenAccount[records.General.TokenAccount])
		if err != nil {
			log.WithError(err).Warn("failure getting proto account info")
			return nil, status.Error(codes.Internal, "")
		}

		tokenAccountInfos[records.General.TokenAccount] = proto
	}

	if len(tokenAccountInfos) == 0 {
		return &accountpb.GetTokenAccountInfosResponse{
			Result: accountpb.GetTokenAccountInfosResponse_NOT_FOUND,
		}, nil
	}

	resp := &accountpb.GetTokenAccountInfosResponse{
		Result:            accountpb.GetTokenAccountInfosResponse_OK,
		TokenAccountInfos: tokenAccountInfos,
		NextPoolIndex:     uint64(nextPoolIndex),
	}

	// Is this a gift card in a terminal state that we can cache?
	if len(tokenAccountInfos) == 1 && filteredRecords[0].General.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		tokenAccountInfo := tokenAccountInfos[filteredRecords[0].General.TokenAccount]

		switch tokenAccountInfo.ClaimState {
		case accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED, accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED:
			giftCardCacheByOwner.Insert(owner.PublicKey().ToBase58(), resp, 1)
		}
	}

	resp, err = s.addRequestingOwnerMetadata(ctx, resp, requestingOwner)
	if err != nil {
		log.WithError(err).Warn("failure adding requesting owner metadata")
		return nil, status.Error(codes.Internal, "")
	}
	return resp, nil
}

// fetchBalances optimally routes and batches balance calculations for a set of
// account records.
func (s *server) fetchBalances(ctx context.Context, allAccountRecords []*common.AccountRecords) (map[string]*balanceMetadata, error) {
	balanceMetadataByTokenAccount := make(map[string]*balanceMetadata)

	var mangedByCodeRecords []*common.AccountRecords
	for _, accountRecords := range allAccountRecords {
		if accountRecords.IsManagedByCode(ctx) {
			mangedByCodeRecords = append(mangedByCodeRecords, accountRecords)
		} else {
			// Don't calculate a balance for now, since the caching strategy
			// is not possible.
			balanceMetadataByTokenAccount[accountRecords.General.TokenAccount] = &balanceMetadata{
				value:  0,
				source: accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN,
			}
		}
	}
	balancesByTokenAccount, err := balance.BatchCalculateFromCacheWithAccountRecords(ctx, s.data, mangedByCodeRecords...)
	if err != nil {
		return nil, err
	}
	for tokenAccount, quarks := range balancesByTokenAccount {
		balanceMetadataByTokenAccount[tokenAccount] = &balanceMetadata{
			value:  quarks,
			source: accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
		}
	}

	// Any accounts that aren't Timelock can be deferred to the blockchain
	//
	// todo: support batching
	for _, accountRecords := range allAccountRecords {
		if accountRecords.IsTimelock() {
			continue
		}

		tokenAccount, err := common.NewAccountFromPublicKeyString(accountRecords.General.TokenAccount)
		if err != nil {
			return nil, err
		}

		quarks, balanceSource, err := balance.CalculateFromBlockchain(ctx, s.data, tokenAccount)
		if err != nil {
			return nil, err
		}
		var protoBalanceSource accountpb.TokenAccountInfo_BalanceSource
		switch balanceSource {
		case balance.BlockchainSource:
			protoBalanceSource = accountpb.TokenAccountInfo_BALANCE_SOURCE_BLOCKCHAIN
		case balance.CacheSource:
			protoBalanceSource = accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE
		default:
			protoBalanceSource = accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN
		}
		balanceMetadataByTokenAccount[tokenAccount.PublicKey().ToBase58()] = &balanceMetadata{
			value:  quarks,
			source: protoBalanceSource,
		}
	}

	return balanceMetadataByTokenAccount, nil
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

	mintAccount, err := common.NewAccountFromPublicKeyString(records.General.MintAccount)
	if err != nil {
		return nil, err
	}

	// todo: We don't yet handle the closing state
	var managementState accountpb.TokenAccountInfo_ManagementState
	if !records.IsTimelock() {
		managementState = accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE
	} else {
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
	}

	blockchainState := accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST
	if records.IsTimelock() && records.Timelock.ExistsOnBlockchain() {
		blockchainState = accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS
	}
	if !records.IsTimelock() {
		blockchainState = accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_UNKNOWN
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
				claimState = accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED
			}
		case action.ErrActionNotFound:
		default:
			return nil, err
		}

		// Gift cards that are close to the auto-return window are marked as expired in
		// a consistent manner as SubmitIntent to avoid race conditions with the auto-return.
		if time.Since(records.General.CreatedAt) >= async_account.GiftCardExpiry-time.Minute {
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
		ClaimState:           claimState,
		OriginalExchangeData: originalExchangeData,
		Mint:                 mintAccount.ToProto(),
		CreatedAt:            timestamppb.New(records.General.CreatedAt),
	}, nil
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
		Currency:     string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency),
		ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
		NativeAmount: intentRecord.SendPublicPaymentMetadata.NativeAmount,
		Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
		Mint:         common.CoreMintAccount.ToProto(),
	}, nil
}

func (s *server) addRequestingOwnerMetadata(ctx context.Context, resp *accountpb.GetTokenAccountInfosResponse, requestingOwner *common.Account) (*accountpb.GetTokenAccountInfosResponse, error) {
	if requestingOwner == nil {
		return resp, nil
	}

	cloned := proto.Clone(resp).(*accountpb.GetTokenAccountInfosResponse)

	for _, ai := range cloned.TokenAccountInfos {
		switch ai.AccountType {
		case commonpb.AccountType_REMOTE_SEND_GIFT_CARD:
			giftCardVaultAccount, err := common.NewAccountFromProto(ai.Address)
			if err != nil {
				return nil, err
			}

			intentRecord, err := s.data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVaultAccount.PublicKey().ToBase58())
			if err != nil {
				return nil, err
			}

			if intentRecord.InitiatorOwnerAccount == requestingOwner.PublicKey().ToBase58() {
				ai.IsGiftCardIssuer = true
			}
		}
	}

	return cloned, nil
}

func (s *server) updateCachedResponse(resp *accountpb.GetTokenAccountInfosResponse) {
	for _, ai := range resp.TokenAccountInfos {
		switch ai.AccountType {
		case commonpb.AccountType_REMOTE_SEND_GIFT_CARD:
			// Transition any gift card records to expired if we elapsed the expiry window
			if time.Since(ai.CreatedAt.AsTime()) >= async_account.GiftCardExpiry-time.Minute {
				ai.ClaimState = accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED
				ai.BalanceSource = accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE
				ai.Balance = 0
			}
		}
	}
}
