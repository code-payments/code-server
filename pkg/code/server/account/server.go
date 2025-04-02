package account

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountpb "github.com/code-payments/code-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/cache"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
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

func (s *server) LinkAdditionalAccounts(ctx context.Context, req *accountpb.LinkAdditionalAccountsRequest) (*accountpb.LinkAdditionalAccountsResponse, error) {
	log := s.log.WithField("method", "LinkAdditionalAccounts")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	swapAuthority, err := common.NewAccountFromProto(req.SwapAuthority)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("swap_authority_account", swapAuthority.PublicKey().ToBase58())

	usdcAta, err := swapAuthority.ToAssociatedTokenAccount(common.UsdcMintAccount)
	if err != nil {
		log.WithError(err).Warn("failure deriving usdc ata address")
		return nil, status.Error(codes.Internal, "")
	}

	signatures := req.Signatures
	req.Signatures = nil
	for i, signer := range []*common.Account{owner, swapAuthority} {
		if err := s.auth.Authenticate(ctx, signer, req, signatures[i]); err != nil {
			return nil, err
		}
	}

	// Validate the owner account

	_, err = s.data.GetLatestIntentByInitiatorAndType(ctx, intent.OpenAccounts, owner.PublicKey().ToBase58())
	if err == intent.ErrIntentNotFound {
		return &accountpb.LinkAdditionalAccountsResponse{
			Result: accountpb.LinkAdditionalAccountsResponse_DENIED,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure checking if owner has opened accounts")
		return nil, status.Error(codes.Internal, "")
	}

	// Validate the swap authority account

	var isInvalidAccount bool
	var recordExists bool

	// Swap account must be derived from the user's 12 words
	if owner.PublicKey().ToBase58() == swapAuthority.PublicKey().ToBase58() {
		isInvalidAccount = true
	}

	existingAccountInfoRecord, err := s.data.GetAccountInfoByAuthorityAddress(ctx, swapAuthority.PublicKey().ToBase58())
	switch err {
	case nil:
		// Attempting to link an account not used for swaps
		if existingAccountInfoRecord.AccountType != commonpb.AccountType_SWAP {
			isInvalidAccount = true
		}

		if existingAccountInfoRecord.OwnerAccount != owner.PublicKey().ToBase58() {
			// Attempting to link an account owned by someone else
			isInvalidAccount = true
		} else if existingAccountInfoRecord.TokenAccount != usdcAta.PublicKey().ToBase58() {
			// Attempting to link an account with an authority for something that's
			// not a USDC ATA
			isInvalidAccount = true
		} else {
			// Otherwise, this RPC is a no-op. Accounts match exactly.
			recordExists = true
		}
	case account.ErrAccountInfoNotFound:
	default:
		log.WithError(err).Warn("failure checking existing account info record")
		return nil, status.Error(codes.Internal, "")
	}

	if isInvalidAccount {
		return &accountpb.LinkAdditionalAccountsResponse{
			Result: accountpb.LinkAdditionalAccountsResponse_INVALID_ACCOUNT,
		}, nil
	}

	// Link account after validation is complete

	if !recordExists {
		accountInfoRecord := &account.Record{
			OwnerAccount:     owner.PublicKey().ToBase58(),
			AuthorityAccount: swapAuthority.PublicKey().ToBase58(),
			TokenAccount:     usdcAta.PublicKey().ToBase58(),
			MintAccount:      common.UsdcMintAccount.PublicKey().ToBase58(),
			AccountType:      commonpb.AccountType_SWAP,
			Index:            0,
			CreatedAt:        time.Now(),
		}
		err = s.data.CreateAccountInfo(ctx, accountInfoRecord)
		if err != nil {
			log.WithError(err).Warn("failure creating account info record")
			return nil, status.Error(codes.Internal, "")
		}
	}

	return &accountpb.LinkAdditionalAccountsResponse{
		Result: accountpb.LinkAdditionalAccountsResponse_OK,
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
	balanceMetadataByTokenAccount, err := s.fetchBalances(ctx, recordsByType)
	if err != nil {
		log.WithError(err).Warn("failure fetching balances")
		return nil, status.Error(codes.Internal, "")
	}

	// Construct token account info
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

// fetchBalances optimally routes and batches balance calculations for a set of
// account records.
func (s *server) fetchBalances(ctx context.Context, recordsByType map[commonpb.AccountType][]*common.AccountRecords) (map[string]*balanceMetadata, error) {
	balanceMetadataByTokenAccount := make(map[string]*balanceMetadata)

	// Pre-privacy accounts are not supported by the new batching stuff
	if _, ok := recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022]; ok {
		tokenAccount, err := common.NewAccountFromPublicKeyString(recordsByType[commonpb.AccountType_LEGACY_PRIMARY_2022][0].General.TokenAccount)
		if err != nil {
			return nil, err
		}

		quarks, err := balance.CalculateFromCache(ctx, s.data, tokenAccount)
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
			return nil, err
		}
	}

	// Post-privacy Timelock account balances can be fetched using a batched method from cache
	var batchedAccountRecords []*common.AccountRecords
	for accountType, batchAccountRecords := range recordsByType {
		if accountType == commonpb.AccountType_LEGACY_PRIMARY_2022 {
			continue
		}

		for _, accountRecords := range batchAccountRecords {
			if accountRecords.IsManagedByCode(ctx) {
				batchedAccountRecords = append(batchedAccountRecords, accountRecords)
			} else {
				// Don't calculate a balance for now, since the caching strategy
				// is not possible.
				balanceMetadataByTokenAccount[accountRecords.General.TokenAccount] = &balanceMetadata{
					value:  0,
					source: accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN,
				}
			}
		}
	}
	balancesByTokenAccount, err := balance.BatchCalculateFromCacheWithAccountRecords(ctx, s.data, batchedAccountRecords...)
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
	for _, batchAccountRecords := range recordsByType {
		for _, accountRecords := range batchAccountRecords {
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
	/*
		if records.General.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
			originalExchangeData, err = s.getOriginalGiftCardExchangeData(ctx, records)
			if err != nil {
				return nil, err
			}
		}
	*/

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
		Mint:                 mintAccount.ToProto(),
		CreatedAt:            timestamppb.New(records.General.CreatedAt),
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

/*
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
*/
