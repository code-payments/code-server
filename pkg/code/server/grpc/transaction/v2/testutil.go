package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/onramp"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	memory_device_verifier "github.com/code-payments/code-server/pkg/device/memory"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	memory_push "github.com/code-payments/code-server/pkg/push/memory"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/solana/system"
	timelock_token_legacy "github.com/code-payments/code-server/pkg/solana/timelock/legacy_2022"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

const (
	defaultTestThirdPartyFeeBps = 249 // 2.49%
)

// todo: Make working with different timelock versions easier

func setupTestEnv(t *testing.T, serverOverrides *testOverrides) (serverTestEnv, phoneTestEnv, phoneTestEnv, func()) {
	var err error

	if serverOverrides.clientReceiveTimeout == 0 {
		serverOverrides.clientReceiveTimeout = defaultClientReceiveTimeout
	}

	db := code_data.NewTestDataProvider()

	serverEnv := serverTestEnv{
		ctx:                   context.Background(),
		data:                  db,
		subsidizer:            testutil.SetupRandomSubsidizer(t, db),
		treasuryPoolByAddress: make(map[string]*treasury.Record),
		treasuryPoolByBucket:  make(map[uint64]*treasury.Record),
		merkleTreeByTreasury:  make(map[string]*merkletree.MerkleTree),
	}

	serverOverrides.feeCollectorTokenPublicKey = testutil.NewRandomAccount(t).PublicKey().ToBase58()

	serverOverrides.treasuryPoolOneKinBucket = "test-pool-one-kin"
	serverOverrides.treasuryPoolTenKinBucket = "test-pool-ten-kin"
	serverOverrides.treasuryPoolHundredKinBucket = "test-pool-hundred-kin"
	serverOverrides.treasuryPoolThousandKinBucket = "test-pool-thousand-kin"
	serverOverrides.treasuryPoolTenThousandKinBucket = "test-pool-ten-thousand-kin"
	serverOverrides.treasuryPoolHundredThousandKinBucket = "test-pool-hundred-thousand-kin"
	serverOverrides.treasuryPoolMillionKinBucket = "test-pool-million-kin"
	for bucket, treasuryPoolName := range map[uint64]string{
		kin.ToQuarks(1):         serverOverrides.treasuryPoolOneKinBucket,
		kin.ToQuarks(10):        serverOverrides.treasuryPoolTenKinBucket,
		kin.ToQuarks(100):       serverOverrides.treasuryPoolHundredKinBucket,
		kin.ToQuarks(1_000):     serverOverrides.treasuryPoolThousandKinBucket,
		kin.ToQuarks(10_000):    serverOverrides.treasuryPoolTenThousandKinBucket,
		kin.ToQuarks(100_000):   serverOverrides.treasuryPoolHundredThousandKinBucket,
		kin.ToQuarks(1_000_000): serverOverrides.treasuryPoolMillionKinBucket,
	} {
		treasuryPoolRecord := &treasury.Record{
			DataVersion: splitter_token.DataVersion1,

			Name: treasuryPoolName,

			Address: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Bump:    uint8(100 + len(serverEnv.treasuryPoolByAddress)),

			Vault:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			VaultBump: uint8(200 + len(serverEnv.treasuryPoolByAddress)),

			Authority: serverEnv.subsidizer.PublicKey().ToBase58(),

			MerkleTreeLevels: 63,

			CurrentIndex:    1,
			HistoryListSize: 2,

			SolanaBlock: 123,

			State: treasury.TreasuryPoolStateAvailable,
		}
		serverEnv.treasuryPoolByAddress[treasuryPoolRecord.Address] = treasuryPoolRecord
		serverEnv.treasuryPoolByBucket[bucket] = treasuryPoolRecord

		merkleTree, err := serverEnv.data.InitializeNewMerkleTree(
			serverEnv.ctx,
			treasuryPoolRecord.Name,
			treasuryPoolRecord.MerkleTreeLevels,
			[]merkletree.Seed{[]byte(treasuryPoolRecord.Name)},
			false,
		)
		require.NoError(t, err)
		serverEnv.merkleTreeByTreasury[treasuryPoolRecord.Address] = merkleTree

		for i := 0; i < int(treasuryPoolRecord.HistoryListSize); i++ {
			hasher := sha256.New()
			hasher.Write([]byte(fmt.Sprintf("root%d", i)))
			recentRoot := hasher.Sum(nil)

			treasuryPoolRecord.HistoryList = append(treasuryPoolRecord.HistoryList, hex.EncodeToString(recentRoot))
		}
		require.NoError(t, serverEnv.data.SaveTreasuryPool(serverEnv.ctx, treasuryPoolRecord))

		require.NoError(t, serverEnv.data.SaveTreasuryPoolFunding(serverEnv.ctx, &treasury.FundingHistoryRecord{
			Vault:         treasuryPoolRecord.Vault,
			DeltaQuarks:   int64(kin.ToQuarks(1_000_000_000)),
			TransactionId: fmt.Sprintf("%s-initial-funding", treasuryPoolRecord.Name),
			State:         treasury.FundingStateConfirmed,
			CreatedAt:     time.Now(),
		}))
	}

	cachedTreasuryMetadatas = make(map[string]*cachedTreasuryMetadata)
	cachedMerkleTrees = make(map[string]*refreshingMerkleTree)

	require.NoError(t, serverEnv.data.ImportExchangeRates(serverEnv.ctx, &currency.MultiRateRecord{
		Time: exchange_rate_util.GetLatestExchangeRateTime(),
		Rates: map[string]float64{
			"usd": 0.1,
			"cad": 0.05,
		},
	}))

	grpcClientConn, grpcTestServer, err := testutil.NewServer()
	require.NoError(t, err)

	testService := NewTransactionServer(
		db,
		memory_push.NewPushProvider(),
		nil,
		messaging.NewMessagingClient(db),
		nil,
		antispam.NewGuard(db, memory_device_verifier.NewMemoryDeviceVerifier(), nil),
		withManualTestOverrides(serverOverrides),
	)
	grpcTestServer.RegisterService(func(server *grpc.Server) {
		transactionpb.RegisterTransactionServer(server, testService)
	})
	serverEnv.service = testService.(*transactionServer)

	cleanup, err := grpcTestServer.Serve()
	require.NoError(t, err)

	var phoneEnvs []phoneTestEnv
	for i := 0; i < 2; i++ {
		// Force iOS user agent to pass airdrop tests
		iosGrpcClientConn, err := grpc.Dial(grpcClientConn.Target(), grpc.WithInsecure(), grpc.WithUserAgent("Code/iOS/1.0.0"))
		require.NoError(t, err)

		phoneEnv := phoneTestEnv{
			ctx:                    context.Background(),
			client:                 transactionpb.NewTransactionClient(iosGrpcClientConn),
			parentAccount:          testutil.NewRandomAccount(t),
			currentDerivedAccounts: make(map[commonpb.AccountType]*common.Account),
			verifiedPhoneNumber:    fmt.Sprintf("+1800555000%d", i),
			allGiftCardAccounts:    make([]*common.Account, 0),
			directServerAccess:     &serverEnv,
		}

		for _, accountType := range accountTypesToOpen {
			if accountType == commonpb.AccountType_PRIMARY {
				continue
			}

			// Client should properly derive owner accounts
			derived := derivedAccount{
				accountType: accountType,
				index:       0,
				value:       testutil.NewRandomAccount(t),
			}
			phoneEnv.allDerivedAccounts = append(phoneEnv.allDerivedAccounts, derived)
			phoneEnv.currentDerivedAccounts[accountType] = derived.value
		}

		verificationRecord := &phone.Verification{
			PhoneNumber:    phoneEnv.verifiedPhoneNumber,
			OwnerAccount:   phoneEnv.parentAccount.PublicKey().ToBase58(),
			LastVerifiedAt: time.Now(),
			CreatedAt:      time.Now(),
		}
		require.NoError(t, serverEnv.data.SavePhoneVerification(serverEnv.ctx, verificationRecord))

		userIdentityRecord := &user_identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneEnv.verifiedPhoneNumber,
			},
			IsStaffUser: false,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, serverEnv.data.PutUser(serverEnv.ctx, userIdentityRecord))

		// Fund accounts that will be used to pass local simulation
		for _, accountType := range []commonpb.AccountType{
			commonpb.AccountType_PRIMARY,
			commonpb.AccountType_BUCKET_1_KIN,
			commonpb.AccountType_BUCKET_10_KIN,
			commonpb.AccountType_BUCKET_100_KIN,
		} {
			serverEnv.fundAccount(t, phoneEnv.getTimelockVault(t, accountType, 0), kin.ToQuarks(10_000))
		}

		// Simulate a legacy timelock account being created
		legacyTimelockAccounts, err := phoneEnv.parentAccount.GetTimelockAccounts(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
		require.NoError(t, err)
		timelockRecord := legacyTimelockAccounts.ToDBRecord()
		timelockRecord.VaultState = timelock_token_v1.StateLocked
		timelockRecord.Block += 1
		require.NoError(t, serverEnv.data.SaveTimelock(serverEnv.ctx, timelockRecord))

		// Simulate a swap account being created
		swapAuthorityAccount := testutil.NewRandomAccount(t)
		swapAta, err := swapAuthorityAccount.ToAssociatedTokenAccount(common.UsdcMintAccount)
		require.NoError(t, err)
		require.NoError(t, serverEnv.data.CreateAccountInfo(serverEnv.ctx, &account.Record{
			OwnerAccount:     phoneEnv.parentAccount.PublicKey().ToBase58(),
			AuthorityAccount: swapAuthorityAccount.PublicKey().ToBase58(),
			TokenAccount:     swapAta.PublicKey().ToBase58(),
			MintAccount:      common.UsdcMintAccount.PublicKey().ToBase58(),
			AccountType:      commonpb.AccountType_SWAP,
		}))

		phoneEnvs = append(phoneEnvs, phoneEnv)
	}

	return serverEnv, phoneEnvs[0], phoneEnvs[1], cleanup
}

type serverTestEnv struct {
	ctx                   context.Context
	service               *transactionServer
	data                  code_data.Provider
	treasuryPoolByAddress map[string]*treasury.Record
	treasuryPoolByBucket  map[uint64]*treasury.Record
	merkleTreeByTreasury  map[string]*merkletree.MerkleTree
	subsidizer            *common.Account
}

func (s *serverTestEnv) simulateTreasuryPayments(t *testing.T) {
	for {
		cursor := query.EmptyCursor

		commitmentRecords, err := s.data.GetAllCommitmentsByState(s.ctx, commitment.StateUnknown, query.WithCursor(cursor))
		if err == commitment.ErrCommitmentNotFound {
			break
		}
		require.NoError(t, err)

		for _, commitmentRecord := range commitmentRecords {
			if commitmentRecord.State == commitment.StateReadyToOpen {
				continue
			}

			merkleTree := s.merkleTreeByTreasury[commitmentRecord.Pool]

			actionRecord, err := s.data.GetActionById(s.ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
			require.NoError(t, err)

			actionRecord.State = action.StatePending
			require.NoError(t, s.data.UpdateAction(s.ctx, actionRecord))

			leafValue, err := base58.Decode(commitmentRecord.Address)
			require.NoError(t, err)

			require.NoError(t, merkleTree.AddLeaf(s.ctx, leafValue))

			commitmentRecord.State = commitment.StateReadyToOpen
			require.NoError(t, s.data.SaveCommitment(s.ctx, commitmentRecord))

			rootNode, err := merkleTree.GetCurrentRootNode(s.ctx)
			require.NoError(t, err)

			commitmentRecord.State = commitment.StateReadyToOpen
			require.NoError(t, s.data.SaveCommitment(s.ctx, commitmentRecord))

			treasuryPool := s.treasuryPoolByAddress[commitmentRecord.Pool]
			treasuryPool.CurrentIndex = 1
			treasuryPool.HistoryList[1] = rootNode.Hash.String()
			treasuryPool.SolanaBlock += 1000
			require.NoError(t, s.data.SaveTreasuryPool(s.ctx, treasuryPool))
		}
	}

	cachedMerkleTrees = make(map[string]*refreshingMerkleTree)
}

// Saves all possible records, so we can account for various account types
func (s *serverTestEnv) fundAccount(t *testing.T, account *common.Account, quarks uint64) {
	paymentRecord := &payment.Record{
		Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Destination: account.PublicKey().ToBase58(),
		Quantity:    quarks,

		Rendezvous: "",
		IsExternal: true,

		TransactionId: fmt.Sprintf("txn%d", rand.Uint64()),

		ConfirmationState: transaction.ConfirmationFinalized,

		ExchangeCurrency: string(currency_lib.KIN),
		ExchangeRate:     1.0,
		UsdMarketValue:   1.0,

		BlockId: 12345,

		CreatedAt: time.Now(),
	}
	require.NoError(t, s.data.CreatePayment(s.ctx, paymentRecord))

	depositRecord := &deposit.Record{
		Signature:      fmt.Sprintf("txn%d", rand.Uint64()),
		Destination:    account.PublicKey().ToBase58(),
		Amount:         quarks,
		UsdMarketValue: 0.1 * float64(quarks) / float64(kin.QuarksPerKin),

		ConfirmationState: transaction.ConfirmationFinalized,
		Slot:              12345,
	}
	require.NoError(t, s.data.SaveExternalDeposit(s.ctx, depositRecord))
}

func (s *serverTestEnv) simulateExternalDepositHistoryItem(t *testing.T, owner, vault *common.Account, quarks uint64) {
	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.ExternalDeposit,

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		ExternalDepositMetadata: &intent.ExternalDepositMetadata{
			DestinationOwnerAccount: owner.PublicKey().ToBase58(),
			DestinationTokenAccount: vault.PublicKey().ToBase58(),
			Quantity:                quarks,
			UsdMarketValue:          0.1 * float64(quarks) / float64(kin.QuarksPerKin),
		},

		State:     intent.StateConfirmed,
		CreatedAt: time.Now(),
	}
	require.NoError(t, s.data.SaveIntent(s.ctx, intentRecord))

	require.NoError(t, chat_util.SendCashTransactionsExchangeMessage(s.ctx, s.data, intentRecord))
	_, err := chat_util.SendMerchantExchangeMessage(s.ctx, s.data, intentRecord, nil)
	require.NoError(t, err)
}

func (s *serverTestEnv) simulateAllCommitmentsUpgraded(t *testing.T) {
	repaymentDestination := testutil.NewRandomAccount(t).PublicKey().ToBase58()

	var cursor query.Cursor
	for {
		commitmentRecords, err := s.data.GetAllCommitmentsByState(s.ctx, commitment.StateReadyToOpen, query.WithCursor(cursor))
		if err == commitment.ErrCommitmentNotFound {
			return
		}
		require.NoError(t, err)

		for _, commitmentRecord := range commitmentRecords {
			commitmentRecord.RepaymentDivertedTo = &repaymentDestination
			require.NoError(t, s.data.SaveCommitment(s.ctx, commitmentRecord))
		}

		cursor = query.ToCursor(commitmentRecords[len(commitmentRecords)-1].Id)
	}
}

func (s *serverTestEnv) simulateAllTemporaryPrivateTransfersSubmitted(t *testing.T) {
	var cursor query.Cursor
	for {
		fulfillmentRecords, err := s.data.GetAllFulfillmentsByState(s.ctx, fulfillment.StateUnknown, true, query.WithCursor(cursor))
		if err == fulfillment.ErrFulfillmentNotFound {
			return
		}
		require.NoError(t, err)

		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType != fulfillment.TemporaryPrivacyTransferWithAuthority {
				continue
			}

			actionRecord, err := s.data.GetActionById(s.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
			require.NoError(t, err)
			actionRecord.State = action.StateConfirmed
			require.NoError(t, s.data.UpdateAction(s.ctx, actionRecord))

			commitmentRecord, err := s.data.GetCommitmentByAction(s.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
			require.NoError(t, err)
			commitmentRecord.State = commitment.StateClosed
			require.NoError(t, s.data.SaveCommitment(s.ctx, commitmentRecord))

			fulfillmentRecord.State = fulfillment.StateConfirmed
			require.NoError(t, s.data.UpdateFulfillment(s.ctx, fulfillmentRecord))
		}

		cursor = query.ToCursor(fulfillmentRecords[len(fulfillmentRecords)-1].Id)
	}
}

func (s *serverTestEnv) simulatePhoneVerifyingUser(t *testing.T, user phoneTestEnv) {
	verificationRecord := &phone.Verification{
		PhoneNumber:    user.verifiedPhoneNumber,
		OwnerAccount:   user.parentAccount.PublicKey().ToBase58(),
		LastVerifiedAt: time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, s.data.SavePhoneVerification(s.ctx, verificationRecord))
}

func (s *serverTestEnv) generateRandomUnclaimedGiftCard(t *testing.T) *common.Account {
	authority := testutil.NewRandomAccount(t)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	accountInfoRecord := &account.Record{
		AccountType:      commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
		OwnerAccount:     authority.PublicKey().ToBase58(),
		AuthorityAccount: authority.PublicKey().ToBase58(),
		TokenAccount:     timelockAccounts.Vault.PublicKey().ToBase58(),
		MintAccount:      timelockAccounts.Mint.PublicKey().ToBase58(),
	}
	require.NoError(t, s.data.CreateAccountInfo(s.ctx, accountInfoRecord))

	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, s.data.SaveTimelock(s.ctx, timelockRecord))

	s.fundAccount(t, timelockAccounts.Vault, kin.ToQuarks(1_000_000))

	return authority
}

func (s *serverTestEnv) generateAvailableNonce(t *testing.T) *nonce.Record {
	nonceAccount := testutil.NewRandomAccount(t)

	var bh solana.Blockhash
	rand.Read(bh[:])

	nonceKey := &vault.Record{
		PublicKey:  nonceAccount.PublicKey().ToBase58(),
		PrivateKey: nonceAccount.PrivateKey().ToBase58(),
		State:      vault.StateAvailable,
		CreatedAt:  time.Now(),
	}
	nonceRecord := &nonce.Record{
		Address:   nonceAccount.PublicKey().ToBase58(),
		Authority: s.subsidizer.PublicKey().ToBase58(),
		Blockhash: base58.Encode(bh[:]),
		Purpose:   nonce.PurposeClientTransaction,
		State:     nonce.StateAvailable,
	}
	require.NoError(t, s.data.SaveKey(s.ctx, nonceKey))
	require.NoError(t, s.data.SaveNonce(s.ctx, nonceRecord))
	return nonceRecord
}

func (s *serverTestEnv) generateAvailableNonces(t *testing.T, count int) []*nonce.Record {
	var nonces []*nonce.Record
	for i := 0; i < count; i++ {
		nonces = append(nonces, s.generateAvailableNonce(t))
	}
	return nonces
}

func (s *serverTestEnv) phoneVerifyUser(t *testing.T, user phoneTestEnv) {
	verificationRecord := &phone.Verification{
		PhoneNumber:    user.verifiedPhoneNumber,
		OwnerAccount:   user.parentAccount.PublicKey().ToBase58(),
		LastVerifiedAt: time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, s.data.SavePhoneVerification(s.ctx, verificationRecord))
}

func (s *serverTestEnv) simulateTimelockAccountInState(t *testing.T, vault *common.Account, state timelock_token_v1.TimelockState) {
	timelockRecord, err := s.data.GetTimelockByVault(s.ctx, vault.PublicKey().ToBase58())
	require.NoError(t, err)

	timelockRecord.VaultState = state
	timelockRecord.Block += 1
	require.NoError(t, s.data.SaveTimelock(s.ctx, timelockRecord))
}

func (s *serverTestEnv) simulatePercentTreasuryFundsUsed(t *testing.T, treasury string, percent float64) {
	totalAvailable, err := s.data.GetTotalAvailableTreasuryPoolFunds(s.ctx, s.treasuryPoolByAddress[treasury].Vault)
	require.NoError(t, err)

	require.NoError(t, s.data.SaveCommitment(s.ctx, &commitment.Record{
		Pool:           treasury,
		Amount:         uint64(percent * float64(totalAvailable)),
		TreasuryRepaid: false,

		// Below can be set to anything
		DataVersion: splitter_token.DataVersion1,
		Address:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Vault:       testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Intent:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Owner:       testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		RecentRoot:  "rr",
		Transcript:  "transacript",
		Destination: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		State:       commitment.StateUnknown,
	}))

	// Allow worker to pick up
	time.Sleep(100 * time.Millisecond)
}

func (s *serverTestEnv) simulateExpiredGiftCard(t *testing.T, giftCardAccount *common.Account) {
	vault := getTimelockVault(t, giftCardAccount)

	issuedIntentRecord, err := s.data.GetOriginalGiftCardIssuedIntent(s.ctx, vault.PublicKey().ToBase58())
	require.NoError(t, err)

	actionRecord, err := s.data.GetGiftCardAutoReturnAction(s.ctx, vault.PublicKey().ToBase58())
	require.NoError(t, err)

	actionRecord.State = action.StatePending
	require.NoError(t, s.data.UpdateAction(s.ctx, actionRecord))

	require.NoError(t, s.data.SaveIntent(s.ctx, &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.ReceivePaymentsPublicly,

		ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
			Source:   vault.PublicKey().ToBase58(),
			Quantity: kin.ToQuarks(42),

			IsRemoteSend:            true,
			IsReturned:              true,
			IsIssuerVoidingGiftCard: false,

			OriginalExchangeCurrency: "cad",
			OriginalExchangeRate:     0.05,
			OriginalNativeAmount:     2.1,

			UsdMarketValue: 4.2,
		},

		InitiatorOwnerAccount: issuedIntentRecord.InitiatorOwnerAccount,
		InitiatorPhoneNumber:  issuedIntentRecord.InitiatorPhoneNumber,

		State: intent.StateConfirmed,
	}))
}

func (s *serverTestEnv) simulateTwitterRegistration(t *testing.T, username string, destination *common.Account) {
	record := &twitter.Record{
		Username:      username,
		Name:          "name",
		ProfilePicUrl: "url",
		TipAddress:    destination.PublicKey().ToBase58(),
	}
	require.NoError(t, s.data.SaveTwitterUser(s.ctx, record))
}

// Captures the majority of intent submission validation, but highly intent-specific
// edge cases will likely be captured elsewhere
func (s serverTestEnv) assertIntentSubmitted(t *testing.T, intentId string, protoMetadata *transactionpb.Metadata, protoActions []*transactionpb.Action, sourcePhone phoneTestEnv, destinationPhone *phoneTestEnv) {
	s.assertIntentRecordSaved(t, intentId, protoMetadata, sourcePhone, destinationPhone)
	s.assertActionRecordsSaved(t, intentId, protoActions)
	s.assertFulfillmentRecordsSaved(t, intentId, protoActions)
	s.assertIntentSubmittedMessageSaved(t, intentId, protoMetadata)

	s.assertNoncesAreReserved(t, intentId)

	s.assertLatestAccountRecordsSaved(t, sourcePhone)
	if destinationPhone != nil {
		s.assertLatestAccountRecordsSaved(t, *destinationPhone)
	}

	s.assertCommitmentRecordsSaved(t, intentId, protoActions)
}

func (s serverTestEnv) assertIntentNotSubmitted(t *testing.T, intentId string) {
	_, err := s.data.GetIntent(s.ctx, intentId)
	assert.Equal(t, intent.ErrIntentNotFound, err)

	_, err = s.data.GetAllActionsByIntent(s.ctx, intentId)
	assert.Equal(t, action.ErrActionNotFound, err)

	_, err = s.data.GetAllFulfillmentsByIntent(s.ctx, intentId)
	assert.Equal(t, fulfillment.ErrFulfillmentNotFound, err)

	messageRecords, err := s.data.GetMessages(s.ctx, intentId)
	require.NoError(t, err)
	assert.Empty(t, messageRecords)

	s.assertNoNoncesReservedForIntent(t, intentId)
}

func (s serverTestEnv) assertIntentRecordSaved(t *testing.T, intentId string, protoMetadata *transactionpb.Metadata, sourcePhone phoneTestEnv, destinationPhone *phoneTestEnv) {
	intentRecord, err := s.data.GetIntent(s.ctx, intentId)
	require.NoError(t, err)

	assert.Equal(t, intentId, intentRecord.IntentId)
	assert.Equal(t, sourcePhone.parentAccount.PublicKey().ToBase58(), intentRecord.InitiatorOwnerAccount)
	assert.Equal(t, sourcePhone.verifiedPhoneNumber, *intentRecord.InitiatorPhoneNumber)
	assert.Equal(t, intent.StatePending, intentRecord.State)

	switch typed := protoMetadata.Type.(type) {
	case *transactionpb.Metadata_OpenAccounts:
		assert.Equal(t, intent.OpenAccounts, intentRecord.IntentType)
		require.NotNil(t, intentRecord.OpenAccountsMetadata)
	case *transactionpb.Metadata_SendPrivatePayment:
		assert.Equal(t, intent.SendPrivatePayment, intentRecord.IntentType)
		require.NotNil(t, intentRecord.SendPrivatePaymentMetadata)
		if destinationPhone == nil {
			assert.Empty(t, intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount)
		} else {
			assert.Equal(t, destinationPhone.parentAccount.PublicKey().ToBase58(), intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount)
		}
		assert.Equal(t, base58.Encode(typed.SendPrivatePayment.Destination.Value), intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
		assert.Equal(t, typed.SendPrivatePayment.ExchangeData.Quarks, intentRecord.SendPrivatePaymentMetadata.Quantity)
		assert.EqualValues(t, typed.SendPrivatePayment.ExchangeData.Currency, intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency)
		assert.Equal(t, typed.SendPrivatePayment.ExchangeData.ExchangeRate, intentRecord.SendPrivatePaymentMetadata.ExchangeRate)
		assert.Equal(t, 0.1*float64(typed.SendPrivatePayment.ExchangeData.Quarks)/kin.QuarksPerKin, intentRecord.SendPrivatePaymentMetadata.UsdMarketValue)
		assert.Equal(t, typed.SendPrivatePayment.IsWithdrawal, intentRecord.SendPrivatePaymentMetadata.IsWithdrawal)
		assert.Equal(t, typed.SendPrivatePayment.IsRemoteSend, intentRecord.SendPrivatePaymentMetadata.IsRemoteSend)
		assert.Equal(t, typed.SendPrivatePayment.IsTip, intentRecord.SendPrivatePaymentMetadata.IsTip)
	case *transactionpb.Metadata_ReceivePaymentsPrivately:
		assert.Equal(t, intent.ReceivePaymentsPrivately, intentRecord.IntentType)
		require.NotNil(t, intentRecord.ReceivePaymentsPrivatelyMetadata)
		assert.Equal(t, base58.Encode(typed.ReceivePaymentsPrivately.Source.Value), intentRecord.ReceivePaymentsPrivatelyMetadata.Source)
		assert.Equal(t, typed.ReceivePaymentsPrivately.Quarks, intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity)
		assert.Equal(t, typed.ReceivePaymentsPrivately.IsDeposit, intentRecord.ReceivePaymentsPrivatelyMetadata.IsDeposit)
		assert.Equal(t, 0.1*float64(typed.ReceivePaymentsPrivately.Quarks)/kin.QuarksPerKin, intentRecord.ReceivePaymentsPrivatelyMetadata.UsdMarketValue)
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		assert.Equal(t, intent.MigrateToPrivacy2022, intentRecord.IntentType)
		require.NotNil(t, intentRecord.MigrateToPrivacy2022Metadata)
		assert.Equal(t, typed.MigrateToPrivacy_2022.Quarks, intentRecord.MigrateToPrivacy2022Metadata.Quantity)
	case *transactionpb.Metadata_SendPublicPayment:
		assert.Equal(t, intent.SendPublicPayment, intentRecord.IntentType)
		require.NotNil(t, intentRecord.SendPublicPaymentMetadata)
		if destinationPhone == nil {
			assert.Empty(t, intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
		} else {
			assert.Equal(t, destinationPhone.parentAccount.PublicKey().ToBase58(), intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
		}
		assert.Equal(t, base58.Encode(typed.SendPublicPayment.Destination.Value), intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
		assert.Equal(t, typed.SendPublicPayment.ExchangeData.Quarks, intentRecord.SendPublicPaymentMetadata.Quantity)
		assert.EqualValues(t, typed.SendPublicPayment.ExchangeData.Currency, intentRecord.SendPublicPaymentMetadata.ExchangeCurrency)
		assert.Equal(t, typed.SendPublicPayment.ExchangeData.ExchangeRate, intentRecord.SendPublicPaymentMetadata.ExchangeRate)
		assert.Equal(t, 0.1*float64(typed.SendPublicPayment.ExchangeData.Quarks)/kin.QuarksPerKin, intentRecord.SendPublicPaymentMetadata.UsdMarketValue)
		assert.Equal(t, typed.SendPublicPayment.IsWithdrawal, intentRecord.SendPublicPaymentMetadata.IsWithdrawal)
	case *transactionpb.Metadata_ReceivePaymentsPublicly:
		assert.Equal(t, intent.ReceivePaymentsPublicly, intentRecord.IntentType)
		require.NotNil(t, intentRecord.ReceivePaymentsPubliclyMetadata)
		assert.Equal(t, base58.Encode(typed.ReceivePaymentsPublicly.Source.Value), intentRecord.ReceivePaymentsPubliclyMetadata.Source)
		assert.Equal(t, typed.ReceivePaymentsPublicly.Quarks, intentRecord.ReceivePaymentsPubliclyMetadata.Quantity)
		assert.Equal(t, typed.ReceivePaymentsPublicly.IsRemoteSend, intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend)
		assert.False(t, intentRecord.ReceivePaymentsPubliclyMetadata.IsReturned)
		assert.Equal(t, typed.ReceivePaymentsPublicly.IsIssuerVoidingGiftCard, intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard)
		assert.Equal(t, 0.1*float64(typed.ReceivePaymentsPublicly.Quarks)/kin.QuarksPerKin, intentRecord.ReceivePaymentsPubliclyMetadata.UsdMarketValue)

		// todo: These are hard coded for the one phone test environment case. Need to
		//       dynamically fetch the original gift card issued intent
		assert.Equal(t, currency_lib.CAD, intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency)
		assert.Equal(t, 0.05, intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate)
		assert.EqualValues(t, 2.1, intentRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount)
	case *transactionpb.Metadata_EstablishRelationship:
		assert.Equal(t, intent.EstablishRelationship, intentRecord.IntentType)
		require.NotNil(t, intentRecord.EstablishRelationshipMetadata)
		assert.Equal(t, typed.EstablishRelationship.Relationship.GetDomain().Value, intentRecord.EstablishRelationshipMetadata.RelationshipTo)
	default:
		assert.Fail(t, "unhandled intent type")
	}
}

func (s serverTestEnv) assertIntentSubmittedMessageSaved(t *testing.T, intentId string, protoMetadata *transactionpb.Metadata) {
	messageRecords, err := s.data.GetMessages(s.ctx, intentId)
	require.NoError(t, err)
	require.Len(t, messageRecords, 1)

	messageRecord := messageRecords[0]
	assert.Equal(t, intentId, messageRecord.Account)
	assert.NotEqual(t, uuid.Nil, messageRecord.MessageID)

	var savedProtoMessage messagingpb.Message
	require.NoError(t, proto.Unmarshal(messageRecord.Message, &savedProtoMessage))

	assert.Equal(t, messageRecord.MessageID[:], savedProtoMessage.Id.Value)
	require.NotNil(t, savedProtoMessage.GetIntentSubmitted())
	assert.Nil(t, savedProtoMessage.SendMessageRequestSignature)
	assert.True(t, proto.Equal(protoMetadata, savedProtoMessage.GetIntentSubmitted().Metadata))
}

func (s serverTestEnv) assertActionRecordsSaved(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	intentRecord, err := s.data.GetIntent(s.ctx, intentId)
	require.NoError(t, err)

	actionRecords, err := s.data.GetAllActionsByIntent(s.ctx, intentId)
	require.NoError(t, err)

	require.Len(t, actionRecords, len(protoActions))

	for i, protoAction := range protoActions {
		actionRecord := actionRecords[i]

		assert.Equal(t, intentId, actionRecord.Intent)
		assert.Equal(t, intentRecord.IntentType, actionRecord.IntentType)
		assert.EqualValues(t, protoAction.Id, actionRecord.ActionId)
		assert.Equal(t, *intentRecord.InitiatorPhoneNumber, *actionRecord.InitiatorPhoneNumber)

		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			assert.Equal(t, action.OpenAccount, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.OpenAccount.Token.Value), actionRecord.Source)
			assert.Nil(t, actionRecord.Destination)
			assert.Nil(t, actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_CloseEmptyAccount:
			assert.Equal(t, action.CloseEmptyAccount, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.CloseEmptyAccount.Token.Value), actionRecord.Source)
			assert.Nil(t, actionRecord.Destination)
			assert.Nil(t, actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_CloseDormantAccount:
			assert.Equal(t, action.CloseDormantAccount, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.CloseDormantAccount.Token.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.CloseDormantAccount.Destination.Value), *actionRecord.Destination)
			assert.Nil(t, actionRecord.Quantity)
			if typed.CloseDormantAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
				assert.Equal(t, action.StateUnknown, actionRecord.State)
			} else {
				assert.Equal(t, action.StateRevoked, actionRecord.State)
			}
		case *transactionpb.Action_NoPrivacyTransfer:
			assert.Equal(t, action.NoPrivacyTransfer, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.NoPrivacyTransfer.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.NoPrivacyTransfer.Destination.Value), *actionRecord.Destination)
			assert.Equal(t, typed.NoPrivacyTransfer.Amount, *actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_FeePayment:
			assert.Equal(t, action.NoPrivacyTransfer, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.FeePayment.Source.Value), actionRecord.Source)
			if typed.FeePayment.Type == transactionpb.FeePaymentAction_CODE {
				assert.Equal(t, s.service.conf.feeCollectorTokenPublicKey.Get(s.ctx), *actionRecord.Destination)
			} else {
				assert.Equal(t, base58.Encode(typed.FeePayment.Destination.Value), *actionRecord.Destination)
			}
			assert.Equal(t, typed.FeePayment.Amount, *actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_NoPrivacyWithdraw:
			assert.Equal(t, action.NoPrivacyWithdraw, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.NoPrivacyWithdraw.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.NoPrivacyWithdraw.Destination.Value), *actionRecord.Destination)
			assert.Equal(t, typed.NoPrivacyWithdraw.Amount, *actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			assert.Equal(t, action.PrivateTransfer, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.TemporaryPrivacyTransfer.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.TemporaryPrivacyTransfer.Destination.Value), *actionRecord.Destination)
			assert.Equal(t, typed.TemporaryPrivacyTransfer.Amount, *actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		case *transactionpb.Action_TemporaryPrivacyExchange:
			assert.Equal(t, action.PrivateTransfer, actionRecord.ActionType)
			assert.Equal(t, base58.Encode(typed.TemporaryPrivacyExchange.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.TemporaryPrivacyExchange.Destination.Value), *actionRecord.Destination)
			assert.Equal(t, typed.TemporaryPrivacyExchange.Amount, *actionRecord.Quantity)
			assert.Equal(t, action.StatePending, actionRecord.State)
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}

func (s serverTestEnv) assertFulfillmentRecordsSaved(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	intentRecord, err := s.data.GetIntent(s.ctx, intentId)
	require.NoError(t, err)

	for _, protoAction := range protoActions {
		actionRecord, err := s.data.GetActionById(s.ctx, intentId, protoAction.Id)
		require.NoError(t, err)

		fulfillmentRecords, err := s.data.GetAllFulfillmentsByAction(s.ctx, intentId, protoAction.Id)
		if err == fulfillment.ErrFulfillmentNotFound {
			fulfillmentRecords = nil
		} else {
			require.NoError(t, err)
		}

		for _, fulfillmentRecord := range fulfillmentRecords {
			assert.Equal(t, intentId, fulfillmentRecord.Intent)
			assert.Equal(t, intentRecord.IntentType, fulfillmentRecord.IntentType)
			assert.Equal(t, protoAction.Id, fulfillmentRecord.ActionId)
			assert.Equal(t, actionRecord.ActionType, fulfillmentRecord.ActionType)
			assert.Equal(t, *intentRecord.InitiatorPhoneNumber, *fulfillmentRecord.InitiatorPhoneNumber)
			assert.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)
		}

		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			require.Len(t, fulfillmentRecords, 1)

			fulfillmentRecord := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.InitializeLockedTimelockAccount, fulfillmentRecord.FulfillmentType)
			assert.Equal(t, base58.Encode(typed.OpenAccount.Token.Value), actionRecord.Source)
			assert.Nil(t, fulfillmentRecord.Destination)
			assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, fulfillmentRecord.ActionOrderingIndex)
			assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
			assert.Equal(t, typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY, fulfillmentRecord.DisableActiveScheduling)
			s.assertOnDemandTransaction(t, fulfillmentRecord)
		case *transactionpb.Action_CloseEmptyAccount:
			require.Len(t, fulfillmentRecords, 1)

			authorityAccount, err := common.NewAccountFromProto(typed.CloseEmptyAccount.Authority)
			require.NoError(t, err)

			fulfillmentRecord := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.CloseEmptyTimelockAccount, fulfillmentRecord.FulfillmentType)
			assert.Equal(t, base58.Encode(typed.CloseEmptyAccount.Token.Value), actionRecord.Source)
			assert.Nil(t, fulfillmentRecord.Destination)
			assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, fulfillmentRecord.ActionOrderingIndex)
			assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
			assert.Equal(t, intentRecord.IntentType == intent.ReceivePaymentsPrivately, fulfillmentRecord.DisableActiveScheduling)
			s.assertSignedTransaction(t, fulfillmentRecord)
			s.assertNoncedTransaction(t, fulfillmentRecord)
			s.assertExpectedCloseEmptyTimelockAccountTransaction(t, intentRecord.IntentType, fulfillmentRecord, authorityAccount)
		case *transactionpb.Action_CloseDormantAccount:
			if typed.CloseDormantAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
				require.Len(t, fulfillmentRecords, 1)

				authorityAccount, err := common.NewAccountFromProto(typed.CloseDormantAccount.Authority)
				require.NoError(t, err)

				destinationAccount, err := common.NewAccountFromProto(typed.CloseDormantAccount.Destination)
				require.NoError(t, err)

				fulfillmentRecord := fulfillmentRecords[0]
				assert.Equal(t, fulfillment.CloseDormantTimelockAccount, fulfillmentRecord.FulfillmentType)
				assert.Equal(t, base58.Encode(typed.CloseDormantAccount.Token.Value), actionRecord.Source)
				assert.Equal(t, base58.Encode(typed.CloseDormantAccount.Destination.Value), *fulfillmentRecord.Destination)
				assert.EqualValues(t, math.MaxInt64, fulfillmentRecord.IntentOrderingIndex)
				assert.EqualValues(t, 0, fulfillmentRecord.ActionOrderingIndex)
				assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
				assert.True(t, fulfillmentRecord.DisableActiveScheduling)
				s.assertSignedTransaction(t, fulfillmentRecord)
				s.assertNoncedTransaction(t, fulfillmentRecord)
				s.assertExpectedCloseTimelockAccountWithBalanceTransaction(t, intentRecord.IntentType, fulfillmentRecord, authorityAccount, destinationAccount, nil)
			} else {
				assert.Empty(t, fulfillmentRecords)
			}
		case *transactionpb.Action_NoPrivacyTransfer:
			require.Len(t, fulfillmentRecords, 1)

			authorityAccount, err := common.NewAccountFromProto(typed.NoPrivacyTransfer.Authority)
			require.NoError(t, err)

			destinationAccount, err := common.NewAccountFromProto(typed.NoPrivacyTransfer.Destination)
			require.NoError(t, err)

			amount := typed.NoPrivacyTransfer.Amount

			fulfillmentRecord := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.NoPrivacyTransferWithAuthority, fulfillmentRecord.FulfillmentType)
			assert.Equal(t, base58.Encode(typed.NoPrivacyTransfer.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.NoPrivacyTransfer.Destination.Value), *fulfillmentRecord.Destination)
			assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, fulfillmentRecord.ActionOrderingIndex)
			assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
			assert.False(t, fulfillmentRecord.DisableActiveScheduling)
			s.assertSignedTransaction(t, fulfillmentRecord)
			s.assertNoncedTransaction(t, fulfillmentRecord)
			s.assertExpectedTransferWithAuthorityTransaction(t, fulfillmentRecord, authorityAccount, destinationAccount, amount)
		case *transactionpb.Action_FeePayment:
			require.Len(t, fulfillmentRecords, 1)

			authorityAccount, err := common.NewAccountFromProto(typed.FeePayment.Authority)
			require.NoError(t, err)

			var destinationAccount *common.Account
			if typed.FeePayment.Type == transactionpb.FeePaymentAction_CODE {
				destinationAccount, err = common.NewAccountFromPublicKeyString(s.service.conf.feeCollectorTokenPublicKey.Get(s.ctx))
				require.NoError(t, err)
			} else {
				destinationAccount, err = common.NewAccountFromProto(typed.FeePayment.Destination)
				require.NoError(t, err)
			}

			amount := typed.FeePayment.Amount

			fulfillmentRecord := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.NoPrivacyTransferWithAuthority, fulfillmentRecord.FulfillmentType)
			assert.Equal(t, base58.Encode(typed.FeePayment.Source.Value), actionRecord.Source)
			assert.Equal(t, destinationAccount.PublicKey().ToBase58(), *fulfillmentRecord.Destination)
			assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, fulfillmentRecord.ActionOrderingIndex)
			assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
			assert.True(t, fulfillmentRecord.DisableActiveScheduling)
			s.assertSignedTransaction(t, fulfillmentRecord)
			s.assertNoncedTransaction(t, fulfillmentRecord)
			s.assertExpectedTransferWithAuthorityTransaction(t, fulfillmentRecord, authorityAccount, destinationAccount, amount)
		case *transactionpb.Action_NoPrivacyWithdraw:
			require.Len(t, fulfillmentRecords, 1)

			authorityAccount, err := common.NewAccountFromProto(typed.NoPrivacyWithdraw.Authority)
			require.NoError(t, err)

			destinationAccount, err := common.NewAccountFromProto(typed.NoPrivacyWithdraw.Destination)
			require.NoError(t, err)

			var additionalMemo *string
			if intentRecord.IntentType == intent.SendPrivatePayment && intentRecord.SendPrivatePaymentMetadata.IsTip {
				tipMemo, err := transaction_util.GetTipMemoValue(intentRecord.SendPrivatePaymentMetadata.TipMetadata.Platform, intentRecord.SendPrivatePaymentMetadata.TipMetadata.Username)
				require.NoError(t, err)
				additionalMemo = &tipMemo
			}

			fulfillmentRecord := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.NoPrivacyWithdraw, fulfillmentRecord.FulfillmentType)
			assert.Equal(t, base58.Encode(typed.NoPrivacyWithdraw.Source.Value), actionRecord.Source)
			assert.Equal(t, base58.Encode(typed.NoPrivacyWithdraw.Destination.Value), *fulfillmentRecord.Destination)
			assert.Equal(t, intentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, fulfillmentRecord.ActionOrderingIndex)
			assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
			assert.Equal(t, intentRecord.IntentType == intent.SendPrivatePayment, fulfillmentRecord.DisableActiveScheduling)
			s.assertSignedTransaction(t, fulfillmentRecord)
			s.assertNoncedTransaction(t, fulfillmentRecord)
			s.assertExpectedCloseTimelockAccountWithBalanceTransaction(t, intentRecord.IntentType, fulfillmentRecord, authorityAccount, destinationAccount, additionalMemo)
		case *transactionpb.Action_TemporaryPrivacyTransfer, *transactionpb.Action_TemporaryPrivacyExchange:
			require.Len(t, fulfillmentRecords, 2)

			var authorityAccount, sourceAccount, destinationAccount *common.Account
			var amount uint64
			var isExchange bool
			if protoAction.GetTemporaryPrivacyTransfer() != nil {
				authorityAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyTransfer().Authority)
				require.NoError(t, err)
				sourceAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyTransfer().Source)
				require.NoError(t, err)
				destinationAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyTransfer().Destination)
				require.NoError(t, err)
				amount = protoAction.GetTemporaryPrivacyTransfer().Amount
			} else {
				authorityAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyExchange().Authority)
				require.NoError(t, err)
				sourceAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyExchange().Source)
				require.NoError(t, err)
				destinationAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyExchange().Destination)
				require.NoError(t, err)
				amount = protoAction.GetTemporaryPrivacyExchange().Amount
				isExchange = true
			}

			commitmentRecord, err := s.data.GetCommitmentByAction(s.ctx, intentId, actionRecord.ActionId)
			require.NoError(t, err)

			commitmentVaultAccount, err := common.NewAccountFromPublicKeyString(commitmentRecord.Vault)
			require.NoError(t, err)

			expectedTreasuryVault := s.getExpectedTreasury(t, amount).Vault

			treasuryToDestination := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.TransferWithCommitment, treasuryToDestination.FulfillmentType)
			assert.Equal(t, expectedTreasuryVault, treasuryToDestination.Source)
			assert.Equal(t, destinationAccount.PublicKey().ToBase58(), *treasuryToDestination.Destination)
			assert.Equal(t, intentRecord.Id, treasuryToDestination.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, treasuryToDestination.ActionOrderingIndex)
			assert.EqualValues(t, 0, treasuryToDestination.FulfillmentOrderingIndex)
			assert.Equal(t, isExchange || intentRecord.IntentType != intent.SendPrivatePayment || !intentRecord.SendPrivatePaymentMetadata.IsWithdrawal, treasuryToDestination.DisableActiveScheduling)
			s.assertOnDemandTransaction(t, treasuryToDestination)

			sourceToCommitment := fulfillmentRecords[1]
			assert.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, sourceToCommitment.FulfillmentType)
			assert.Equal(t, sourceAccount.PublicKey().ToBase58(), sourceToCommitment.Source)
			assert.Equal(t, commitmentRecord.Vault, *sourceToCommitment.Destination)
			assert.Equal(t, intentRecord.Id, sourceToCommitment.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, sourceToCommitment.ActionOrderingIndex)
			assert.EqualValues(t, 2000, sourceToCommitment.FulfillmentOrderingIndex)
			assert.True(t, sourceToCommitment.DisableActiveScheduling)
			s.assertSignedTransaction(t, sourceToCommitment)
			s.assertNoncedTransaction(t, sourceToCommitment)
			s.assertExpectedTransferWithAuthorityTransaction(t, sourceToCommitment, authorityAccount, commitmentVaultAccount, amount)
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			require.Len(t, fulfillmentRecords, 3)

			treasuryToDestination := fulfillmentRecords[0]
			assert.Equal(t, fulfillment.TransferWithCommitment, treasuryToDestination.FulfillmentType)

			sourceToCommitmentWithTempPrivacy := fulfillmentRecords[1]
			assert.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, sourceToCommitmentWithTempPrivacy.FulfillmentType)

			accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(s.ctx, sourceToCommitmentWithTempPrivacy.Source)
			require.NoError(t, err)

			commitmentRecord, err := s.data.GetCommitmentByAction(s.ctx, intentId, actionRecord.ActionId)
			require.NoError(t, err)

			authorityAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.AuthorityAccount)
			require.NoError(t, err)

			newCommitmentVaultAccount, err := common.NewAccountFromPublicKeyString(*commitmentRecord.RepaymentDivertedTo)
			require.NoError(t, err)

			sourceToCommitmentWithPermanentPrivacy := fulfillmentRecords[2]
			assert.Equal(t, fulfillment.PermanentPrivacyTransferWithAuthority, sourceToCommitmentWithPermanentPrivacy.FulfillmentType)
			assert.Equal(t, sourceToCommitmentWithTempPrivacy.Source, sourceToCommitmentWithPermanentPrivacy.Source)
			assert.Equal(t, newCommitmentVaultAccount.PublicKey().ToBase58(), *sourceToCommitmentWithPermanentPrivacy.Destination)
			assert.Equal(t, intentRecord.Id, sourceToCommitmentWithPermanentPrivacy.IntentOrderingIndex)
			assert.Equal(t, actionRecord.ActionId, sourceToCommitmentWithPermanentPrivacy.ActionOrderingIndex)
			assert.EqualValues(t, 1000, sourceToCommitmentWithPermanentPrivacy.FulfillmentOrderingIndex)
			assert.False(t, sourceToCommitmentWithPermanentPrivacy.DisableActiveScheduling)
			s.assertSignedTransaction(t, sourceToCommitmentWithPermanentPrivacy)
			s.assertNoncedTransaction(t, sourceToCommitmentWithPermanentPrivacy)
			s.assertExpectedTransferWithAuthorityTransaction(t, sourceToCommitmentWithPermanentPrivacy, authorityAccount, newCommitmentVaultAccount, commitmentRecord.Amount)
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}

// todo: do we just always set this up?
func (s *serverTestEnv) setupAirdropper(t *testing.T, initialFunds uint64) *common.Account {
	require.Nil(t, s.service.airdropper)

	owner := testutil.NewRandomAccount(t)

	timelockAccounts, err := owner.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, s.data.SaveTimelock(s.ctx, timelockRecord))

	s.service.airdropper = timelockAccounts
	s.fundAccount(t, getTimelockVault(t, owner), initialFunds)

	return owner
}

func (s *serverTestEnv) assertAirdroppedFirstKin(t *testing.T, phone phoneTestEnv) {
	airdropIntentId := GetNewAirdropIntentId(AirdropTypeGetFirstKin, phone.parentAccount.PublicKey().ToBase58())
	s.assertAirdropped(t, phone, AirdropTypeGetFirstKin, airdropIntentId, 1.0)
}

func (s *serverTestEnv) assertNotAirdroppedFirstKin(t *testing.T, phone phoneTestEnv) {
	airdropIntentId := GetNewAirdropIntentId(AirdropTypeGetFirstKin, phone.parentAccount.PublicKey().ToBase58())
	_, err := s.data.GetIntent(s.ctx, airdropIntentId)
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func (s *serverTestEnv) assertAirdroppedForGivingFirstKin(t *testing.T, phone phoneTestEnv, intentId string) {
	airdropIntentId := GetNewAirdropIntentId(AirdropTypeGiveFirstKin, intentId)
	s.assertAirdropped(t, phone, AirdropTypeGiveFirstKin, airdropIntentId, 5.0)
}

func (s *serverTestEnv) assertNotAirdroppedForGivingFirstKin(t *testing.T, intentId string) {
	airdropIntentId := GetNewAirdropIntentId(AirdropTypeGiveFirstKin, intentId)
	_, err := s.data.GetIntent(s.ctx, airdropIntentId)
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func (s serverTestEnv) assertAirdropped(t *testing.T, phone phoneTestEnv, airdropType AirdropType, intentId string, usdValue float64) {
	airdropIntentRecord, err := s.data.GetIntent(s.ctx, intentId)
	require.NoError(t, err)

	expectedQuarks := kin.ToQuarks(uint64(usdValue / 0.1))
	expectedQuarks += kin.ToQuarks(1)

	assert.Equal(t, airdropIntentRecord.IntentType, intent.SendPublicPayment)
	assert.Equal(t, s.service.airdropper.VaultOwner.PublicKey().ToBase58(), airdropIntentRecord.InitiatorOwnerAccount)
	assert.Nil(t, airdropIntentRecord.InitiatorPhoneNumber)
	assert.Equal(t, phone.parentAccount.PublicKey().ToBase58(), airdropIntentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	assert.Equal(t, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58(), airdropIntentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
	assert.Equal(t, expectedQuarks, airdropIntentRecord.SendPublicPaymentMetadata.Quantity)
	assert.Equal(t, currency_lib.USD, airdropIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency)
	assert.Equal(t, 0.1, airdropIntentRecord.SendPublicPaymentMetadata.ExchangeRate)
	assert.Equal(t, usdValue, airdropIntentRecord.SendPublicPaymentMetadata.NativeAmount)
	assert.Equal(t, usdValue, airdropIntentRecord.SendPublicPaymentMetadata.UsdMarketValue)
	assert.True(t, airdropIntentRecord.SendPublicPaymentMetadata.IsWithdrawal)
	assert.Equal(t, intent.StatePending, airdropIntentRecord.State)

	airdropActionRecords, err := s.data.GetAllActionsByIntent(s.ctx, airdropIntentRecord.IntentId)
	require.NoError(t, err)
	require.Len(t, airdropActionRecords, 1)
	airdropActionRecord := airdropActionRecords[0]

	assert.Equal(t, airdropIntentRecord.IntentId, airdropActionRecord.Intent)
	assert.Equal(t, airdropIntentRecord.IntentType, airdropActionRecord.IntentType)
	assert.EqualValues(t, 0, airdropActionRecord.ActionId)
	assert.Equal(t, action.NoPrivacyTransfer, airdropActionRecord.ActionType)
	assert.Equal(t, s.service.airdropper.Vault.PublicKey().ToBase58(), airdropActionRecord.Source)
	assert.Equal(t, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58(), *airdropActionRecord.Destination)
	assert.Equal(t, expectedQuarks, *airdropActionRecord.Quantity)
	assert.Nil(t, airdropActionRecord.InitiatorPhoneNumber)
	assert.Equal(t, action.StatePending, airdropActionRecord.State)

	airdropFulfillmentRecords, err := s.data.GetAllFulfillmentsByAction(s.ctx, airdropActionRecord.Intent, 0)
	require.NoError(t, err)
	require.Len(t, airdropFulfillmentRecords, 1)
	airdropFulfillmentRecord := airdropFulfillmentRecords[0]

	assert.Equal(t, airdropIntentRecord.IntentId, airdropFulfillmentRecord.Intent)
	assert.Equal(t, airdropIntentRecord.IntentType, airdropFulfillmentRecord.IntentType)
	assert.Equal(t, airdropFulfillmentRecord.ActionId, airdropFulfillmentRecord.ActionId)
	assert.Equal(t, airdropFulfillmentRecord.ActionType, airdropFulfillmentRecord.ActionType)
	assert.Equal(t, fulfillment.NoPrivacyTransferWithAuthority, airdropFulfillmentRecord.FulfillmentType)
	assert.Equal(t, s.service.airdropper.Vault.PublicKey().ToBase58(), airdropFulfillmentRecord.Source)
	assert.Equal(t, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58(), *airdropFulfillmentRecord.Destination)
	assert.Nil(t, airdropFulfillmentRecord.InitiatorPhoneNumber)
	assert.Equal(t, airdropIntentRecord.Id, airdropFulfillmentRecord.IntentOrderingIndex)
	assert.EqualValues(t, 0, airdropFulfillmentRecord.ActionOrderingIndex)
	assert.EqualValues(t, 0, airdropFulfillmentRecord.FulfillmentOrderingIndex)
	assert.False(t, airdropFulfillmentRecord.DisableActiveScheduling)
	assert.Equal(t, fulfillment.StateUnknown, airdropFulfillmentRecord.State)
	s.assertSignedTransaction(t, airdropFulfillmentRecord)
	s.assertExpectedTransferWithAuthorityTransaction(t, airdropFulfillmentRecord, s.service.airdropper.VaultOwner, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), expectedQuarks)
	s.assertNoncesAreReserved(t, airdropIntentRecord.IntentId)

	messages, err := s.data.GetMessages(s.ctx, airdropIntentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	require.NoError(t, err)

	if airdropType == AirdropTypeGetFirstKin {
		assert.Empty(t, messages)
	} else {
		require.Len(t, messages, 1) // todo: all tests currently do 1 incentive at a time

		message := messages[0]
		idBytes, err := message.MessageID.MarshalBinary()
		require.NoError(t, err)

		assert.Equal(t, airdropIntentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount, message.Account)

		var protoMessage messagingpb.Message
		require.NoError(t, proto.Unmarshal(message.Message, &protoMessage))

		assert.Equal(t, idBytes, protoMessage.Id.Value)
		assert.Nil(t, protoMessage.SendMessageRequestSignature)
		require.NotNil(t, protoMessage.GetAirdropReceived())
		assert.EqualValues(t, airdropType, protoMessage.GetAirdropReceived().AirdropType)
		assert.EqualValues(t, airdropIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency, protoMessage.GetAirdropReceived().ExchangeData.Currency)
		assert.Equal(t, airdropIntentRecord.SendPublicPaymentMetadata.ExchangeRate, protoMessage.GetAirdropReceived().ExchangeData.ExchangeRate)
		assert.Equal(t, airdropIntentRecord.SendPublicPaymentMetadata.NativeAmount, protoMessage.GetAirdropReceived().ExchangeData.NativeAmount)
		assert.Equal(t, airdropIntentRecord.SendPublicPaymentMetadata.Quantity, protoMessage.GetAirdropReceived().ExchangeData.Quarks)
		assert.Equal(t, airdropIntentRecord.CreatedAt.Unix(), protoMessage.GetAirdropReceived().Timestamp.AsTime().Unix())
	}
}

func (s serverTestEnv) assertNoNoncesReserved(t *testing.T) {
	count, err := s.data.GetNonceCountByState(s.ctx, nonce.StateReserved)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}

func (s serverTestEnv) assertNoNoncesReservedForIntent(t *testing.T, intentId string) {
	var cursor query.Cursor
	for {
		nonceRecords, err := s.data.GetAllNonceByState(s.ctx, nonce.StateReserved, query.WithCursor(cursor))
		if err == nonce.ErrNonceNotFound {
			return
		}

		for _, nonceRecord := range nonceRecords {
			fulfillmentRecord, err := s.data.GetFulfillmentBySignature(s.ctx, nonceRecord.Signature)
			require.NoError(t, err)
			assert.NotEqual(t, intentId, fulfillmentRecord.Intent)
		}

		cursor = query.ToCursor(nonceRecords[len(nonceRecords)-1].Id)
	}
}

func (s serverTestEnv) assertNoncesAreReserved(t *testing.T, intentId string) {
	fulfillmentRecords, err := s.data.GetAllFulfillmentsByIntent(s.ctx, intentId)
	require.NoError(t, err)

	usedNonces := make(map[string]struct{})
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.Nonce == nil {
			continue
		}

		_, ok := usedNonces[*fulfillmentRecord.Nonce]
		assert.False(t, ok)
		usedNonces[*fulfillmentRecord.Nonce] = struct{}{}

		nonceRecord, err := s.data.GetNonce(s.ctx, *fulfillmentRecord.Nonce)
		require.NoError(t, err)
		assert.Equal(t, *fulfillmentRecord.Signature, nonceRecord.Signature)
		assert.Equal(t, nonceRecord.Blockhash, *fulfillmentRecord.Blockhash)
		assert.Equal(t, nonce.StateReserved, nonceRecord.State)
	}
}

func (s serverTestEnv) assertNoncesAreUpgraded(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	for _, protoAction := range protoActions {
		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			fulfillmentRecords, err := s.data.GetAllFulfillmentsByAction(s.ctx, intentId, typed.PermanentPrivacyUpgrade.ActionId)
			require.NoError(t, err)

			require.Len(t, fulfillmentRecords, 3)

			sourceToCommitmentWithTempPrivacy := fulfillmentRecords[1]
			assert.Equal(t, fulfillment.TemporaryPrivacyTransferWithAuthority, sourceToCommitmentWithTempPrivacy.FulfillmentType)

			sourceToCommitmentWithPermanentPrivacy := fulfillmentRecords[2]
			assert.Equal(t, fulfillment.PermanentPrivacyTransferWithAuthority, sourceToCommitmentWithPermanentPrivacy.FulfillmentType)

			assert.Equal(t, sourceToCommitmentWithTempPrivacy.Nonce, sourceToCommitmentWithPermanentPrivacy.Nonce)
			assert.Equal(t, sourceToCommitmentWithTempPrivacy.Blockhash, sourceToCommitmentWithPermanentPrivacy.Blockhash)

			nonceRecord, err := s.data.GetNonce(s.ctx, *sourceToCommitmentWithTempPrivacy.Nonce)
			require.NoError(t, err)
			assert.Equal(t, nonce.StateReserved, nonceRecord.State)
			assert.Equal(t, *sourceToCommitmentWithPermanentPrivacy.Signature, nonceRecord.Signature)
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}

// todo: there's duplication of account record check code
func (s serverTestEnv) assertLatestAccountRecordsSaved(t *testing.T, phone phoneTestEnv) {
	accountRecordsByType, err := common.GetLatestTokenAccountRecordsForOwner(s.ctx, s.data, phone.parentAccount)
	require.NoError(t, err)

	for _, accountType := range accountTypesToOpen {
		accountRecords, ok := accountRecordsByType[accountType]
		require.True(t, ok)

		authorityAccount, index := phone.getAuthorityForLatestAccount(t, accountType)

		timelockAccounts, err := authorityAccount.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
		require.NoError(t, err)

		assert.Equal(t, accountType, accountRecords[0].General.AccountType)
		assert.Equal(t, phone.parentAccount.PublicKey().ToBase58(), accountRecords[0].General.OwnerAccount)
		assert.Equal(t, authorityAccount.PublicKey().ToBase58(), accountRecords[0].General.AuthorityAccount)
		assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), accountRecords[0].General.TokenAccount)
		assert.Equal(t, kin.Mint, accountRecords[0].General.MintAccount)
		assert.EqualValues(t, index, accountRecords[0].General.Index)
		assert.Nil(t, accountRecords[0].General.RelationshipTo)
		assert.False(t, accountRecords[0].General.RequiresDepositSync)
		assert.False(t, accountRecords[0].General.RequiresAutoReturnCheck)

		assert.Equal(t, timelock_token_v1.DataVersion1, accountRecords[0].Timelock.DataVersion)
		assert.Equal(t, timelockAccounts.State.PublicKey().ToBase58(), accountRecords[0].Timelock.Address)
		assert.Equal(t, timelockAccounts.StateBump, accountRecords[0].Timelock.Bump)
		assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), accountRecords[0].Timelock.VaultAddress)
		assert.Equal(t, timelockAccounts.VaultBump, accountRecords[0].Timelock.VaultBump)
		assert.Equal(t, authorityAccount.PublicKey().ToBase58(), accountRecords[0].Timelock.VaultOwner)
		assert.Equal(t, timelock_token_v1.StateUnknown, accountRecords[0].Timelock.VaultState)
		assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), accountRecords[0].Timelock.TimeAuthority)
		assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), accountRecords[0].Timelock.CloseAuthority)
		assert.Equal(t, kin.Mint, accountRecords[0].Timelock.Mint)
		assert.Equal(t, timelock_token_v1.DefaultNumDaysLocked, accountRecords[0].Timelock.NumDaysLocked)
		assert.Nil(t, accountRecords[0].Timelock.UnlockAt)
		assert.EqualValues(t, 0, accountRecords[0].Timelock.Block)
	}
}

// todo: there's duplication of account record check code
func (s serverTestEnv) assertRemoteSendGiftCardAccountRecordsSaved(t *testing.T, authorityAccount *common.Account) {
	timelockAccounts, err := authorityAccount.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(s.ctx, timelockAccounts.Vault.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, accountInfoRecord.AccountType)
	assert.Equal(t, authorityAccount.PublicKey().ToBase58(), accountInfoRecord.OwnerAccount)
	assert.Equal(t, authorityAccount.PublicKey().ToBase58(), accountInfoRecord.AuthorityAccount)
	assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), accountInfoRecord.TokenAccount)
	assert.Equal(t, kin.Mint, accountInfoRecord.MintAccount)
	assert.EqualValues(t, 0, accountInfoRecord.Index)
	assert.False(t, accountInfoRecord.RequiresDepositSync)
	assert.True(t, accountInfoRecord.RequiresAutoReturnCheck)

	timelockRecord, err := s.data.GetTimelockByVault(s.ctx, timelockAccounts.Vault.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, timelock_token_v1.DataVersion1, timelockRecord.DataVersion)
	assert.Equal(t, timelockAccounts.State.PublicKey().ToBase58(), timelockRecord.Address)
	assert.Equal(t, timelockAccounts.StateBump, timelockRecord.Bump)
	assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), timelockRecord.VaultAddress)
	assert.Equal(t, timelockAccounts.VaultBump, timelockRecord.VaultBump)
	assert.Equal(t, authorityAccount.PublicKey().ToBase58(), timelockRecord.VaultOwner)
	assert.Equal(t, timelock_token_v1.StateUnknown, timelockRecord.VaultState)
	assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), timelockRecord.TimeAuthority)
	assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), timelockRecord.CloseAuthority)
	assert.Equal(t, kin.Mint, timelockRecord.Mint)
	assert.Equal(t, timelock_token_v1.DefaultNumDaysLocked, timelockRecord.NumDaysLocked)
	assert.Nil(t, timelockRecord.UnlockAt)
	assert.EqualValues(t, 0, timelockRecord.Block)
}

// todo: there's duplication of account record check code
func (s serverTestEnv) assertFirstRelationshipAccountRecordsSaved(t *testing.T, phone phoneTestEnv, relationshipTo string) {
	for _, derivedAccount := range phone.allDerivedAccounts {
		if derivedAccount.accountType != commonpb.AccountType_RELATIONSHIP || *derivedAccount.relationshipTo != relationshipTo {
			continue
		}

		timelockAccounts, err := derivedAccount.value.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
		require.NoError(t, err)

		accountInfoRecord, err := s.data.GetAccountInfoByAuthorityAddress(s.ctx, derivedAccount.value.PublicKey().ToBase58())
		require.NoError(t, err)

		timelockRecord, err := s.data.GetTimelockByVault(s.ctx, timelockAccounts.Vault.PublicKey().ToBase58())
		require.NoError(t, err)

		assert.Equal(t, commonpb.AccountType_RELATIONSHIP, accountInfoRecord.AccountType)
		assert.Equal(t, phone.parentAccount.PublicKey().ToBase58(), accountInfoRecord.OwnerAccount)
		assert.Equal(t, derivedAccount.value.PublicKey().ToBase58(), accountInfoRecord.AuthorityAccount)
		assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), accountInfoRecord.TokenAccount)
		assert.Equal(t, kin.Mint, accountInfoRecord.MintAccount)
		assert.EqualValues(t, 0, accountInfoRecord.Index)
		require.NotNil(t, accountInfoRecord.RelationshipTo)
		assert.Equal(t, *derivedAccount.relationshipTo, *accountInfoRecord.RelationshipTo)
		assert.False(t, accountInfoRecord.RequiresDepositSync)
		assert.False(t, accountInfoRecord.RequiresAutoReturnCheck)

		assert.Equal(t, timelock_token_v1.DataVersion1, timelockRecord.DataVersion)
		assert.Equal(t, timelockAccounts.State.PublicKey().ToBase58(), timelockRecord.Address)
		assert.Equal(t, timelockAccounts.StateBump, timelockRecord.Bump)
		assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBase58(), timelockRecord.VaultAddress)
		assert.Equal(t, timelockAccounts.VaultBump, timelockRecord.VaultBump)
		assert.Equal(t, derivedAccount.value.PublicKey().ToBase58(), timelockRecord.VaultOwner)
		assert.Equal(t, timelock_token_v1.StateUnknown, timelockRecord.VaultState)
		assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), timelockRecord.TimeAuthority)
		assert.Equal(t, s.subsidizer.PublicKey().ToBase58(), timelockRecord.CloseAuthority)
		assert.Equal(t, kin.Mint, timelockRecord.Mint)
		assert.Equal(t, timelock_token_v1.DefaultNumDaysLocked, timelockRecord.NumDaysLocked)
		assert.Nil(t, timelockRecord.UnlockAt)
		assert.EqualValues(t, 0, timelockRecord.Block)

		return
	}
	require.Fail(t, "relationship account not found")
}

func (s serverTestEnv) assertNoRelationshipAccountRecordsSaved(t *testing.T, phone phoneTestEnv, relationshipTo string) {
	for _, derivedAccount := range phone.allDerivedAccounts {
		if derivedAccount.accountType != commonpb.AccountType_RELATIONSHIP || *derivedAccount.relationshipTo != relationshipTo {
			continue
		}

		timelockAccounts, err := derivedAccount.value.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
		require.NoError(t, err)

		_, err = s.data.GetAccountInfoByAuthorityAddress(s.ctx, derivedAccount.value.PublicKey().ToBase58())
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		_, err = s.data.GetTimelockByVault(s.ctx, timelockAccounts.Vault.PublicKey().ToBase58())
		assert.Equal(t, timelock.ErrTimelockNotFound, err)
	}
}

func (s serverTestEnv) assertCommitmentRecordsSaved(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	intentRecord, err := s.data.GetIntent(s.ctx, intentId)
	require.NoError(t, err)

	for _, protoAction := range protoActions {
		switch protoAction.Type.(type) {
		case *transactionpb.Action_TemporaryPrivacyTransfer, *transactionpb.Action_TemporaryPrivacyExchange:
			commitmentRecord, err := s.data.GetCommitmentByAction(s.ctx, intentId, protoAction.Id)
			require.NoError(t, err)

			var amount uint64
			var sourceAccount, destinationAccount *common.Account
			if protoAction.GetTemporaryPrivacyTransfer() != nil {
				amount = protoAction.GetTemporaryPrivacyTransfer().Amount
				sourceAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyTransfer().Source)
				require.NoError(t, err)
				destinationAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyTransfer().Destination)
				require.NoError(t, err)
			} else {
				amount = protoAction.GetTemporaryPrivacyExchange().Amount
				sourceAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyExchange().Source)
				require.NoError(t, err)
				destinationAccount, err = common.NewAccountFromProto(protoAction.GetTemporaryPrivacyExchange().Destination)
				require.NoError(t, err)
			}

			expectedTreasury := s.getExpectedTreasury(t, amount)

			poolAccount, err := common.NewAccountFromPublicKeyString(expectedTreasury.Address)
			require.NoError(t, err)

			recentRootBytes, err := hex.DecodeString(expectedTreasury.GetMostRecentRoot())
			require.NoError(t, err)

			expectedTranscript := getTransript(intentId, protoAction.Id, sourceAccount, destinationAccount, amount)

			expectedAddress, expectedBump, err := splitter_token.GetCommitmentStateAddress(&splitter_token.GetCommitmentStateAddressArgs{
				Pool:        poolAccount.PublicKey().ToBytes(),
				RecentRoot:  recentRootBytes,
				Transcript:  expectedTranscript,
				Destination: destinationAccount.PublicKey().ToBytes(),
				Amount:      amount,
			})
			require.NoError(t, err)

			expectedVaultAddress, expectedVaultBump, err := splitter_token.GetCommitmentVaultAddress(&splitter_token.GetCommitmentVaultAddressArgs{
				Pool:       poolAccount.PublicKey().ToBytes(),
				Commitment: expectedAddress,
			})
			require.NoError(t, err)

			assert.Equal(t, splitter_token.DataVersion1, commitmentRecord.DataVersion)
			assert.Equal(t, base58.Encode(expectedAddress), commitmentRecord.Address)
			assert.Equal(t, expectedBump, commitmentRecord.Bump)
			assert.Equal(t, base58.Encode(expectedVaultAddress), commitmentRecord.Vault)
			assert.Equal(t, expectedVaultBump, commitmentRecord.VaultBump)
			assert.Equal(t, expectedTreasury.Address, commitmentRecord.Pool)
			assert.Equal(t, expectedTreasury.Bump, commitmentRecord.PoolBump)
			assert.Equal(t, expectedTreasury.GetMostRecentRoot(), commitmentRecord.RecentRoot)
			assert.Equal(t, hex.EncodeToString(expectedTranscript), commitmentRecord.Transcript)
			assert.Equal(t, destinationAccount.PublicKey().ToBase58(), commitmentRecord.Destination)
			assert.Equal(t, amount, commitmentRecord.Amount)
			assert.Equal(t, intentId, commitmentRecord.Intent)
			assert.Equal(t, protoAction.Id, commitmentRecord.ActionId)
			assert.Equal(t, intentRecord.InitiatorOwnerAccount, commitmentRecord.Owner)
			assert.False(t, commitmentRecord.TreasuryRepaid)
			assert.Nil(t, commitmentRecord.RepaymentDivertedTo)
			assert.Equal(t, commitment.StateUnknown, commitmentRecord.State)
		}
	}
}

func (s serverTestEnv) assertCommitmentRecordsUpgraded(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	for _, protoAction := range protoActions {
		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			commitmentRecord, err := s.data.GetCommitmentByAction(s.ctx, intentId, typed.PermanentPrivacyUpgrade.ActionId)
			require.NoError(t, err)

			expectedTreasury := s.getExpectedTreasury(t, commitmentRecord.Amount)

			merkleTree, err := s.data.LoadExistingMerkleTree(s.ctx, expectedTreasury.Name, true)
			require.NoError(t, err)

			latestLeafNode, err := merkleTree.GetLastAddedLeafNode(s.ctx)
			require.NoError(t, err)

			expectedCommitmentUpgradedToRecord, err := s.data.GetCommitmentByAddress(s.ctx, base58.Encode(latestLeafNode.LeafValue))
			require.NoError(t, err)

			require.NotNil(t, commitmentRecord.RepaymentDivertedTo)
			assert.Equal(t, expectedCommitmentUpgradedToRecord.Vault, *commitmentRecord.RepaymentDivertedTo)
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}

func (s serverTestEnv) assertPrivacyUpgraded(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	s.assertCommitmentRecordsUpgraded(t, intentId, protoActions)
	s.assertNoncesAreUpgraded(t, intentId, protoActions)
	s.assertFulfillmentRecordsSaved(t, intentId, protoActions)
}

func (s serverTestEnv) assertNoPrivacyUpgrades(t *testing.T, intentId string, protoActions []*transactionpb.Action) {
	for _, protoAction := range protoActions {
		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			fulfillmentRecords, err := s.data.GetAllFulfillmentsByAction(s.ctx, intentId, typed.PermanentPrivacyUpgrade.ActionId)
			require.NoError(t, err)

			assert.Len(t, fulfillmentRecords, 2)
			for _, fulfillmentRecord := range fulfillmentRecords {
				assert.NotEqual(t, fulfillment.PermanentPrivacyTransferWithAuthority, fulfillmentRecord.FulfillmentType)
			}
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}

func (s serverTestEnv) assertNoncedTransaction(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	require.NotNil(t, fulfillmentRecord.Nonce)
	require.NotNil(t, fulfillmentRecord.Blockhash)
	require.NotEmpty(t, fulfillmentRecord.Data)

	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

	assert.Equal(t, base58.Encode(txn.Message.RecentBlockhash[:]), *fulfillmentRecord.Blockhash)

	advanceNonceIxn, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	assert.Equal(t, base58.Encode(advanceNonceIxn.Nonce), *fulfillmentRecord.Nonce)
	assert.EqualValues(t, advanceNonceIxn.Authority, s.subsidizer.PublicKey().ToBytes())
}

func (s serverTestEnv) assertSignedTransaction(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	require.NotNil(t, fulfillmentRecord.Signature)
	require.NotEmpty(t, fulfillmentRecord.Data)

	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

	assert.True(t, int(txn.Message.Header.NumSignatures) < 3)

	var expectedSignatureCount int
	switch fulfillmentRecord.FulfillmentType {
	case fulfillment.CloseEmptyTimelockAccount,
		fulfillment.CloseDormantTimelockAccount,
		fulfillment.NoPrivacyTransferWithAuthority,
		fulfillment.NoPrivacyWithdraw,
		fulfillment.TemporaryPrivacyTransferWithAuthority,
		fulfillment.PermanentPrivacyTransferWithAuthority:
		expectedSignatureCount = 2
	default:
		expectedSignatureCount = 1
	}
	assert.EqualValues(t, expectedSignatureCount, txn.Message.Header.NumSignatures)

	transactionId := ed25519.Sign(s.subsidizer.PrivateKey().ToBytes(), txn.Message.Marshal())
	assert.EqualValues(t, txn.Signatures[0][:], transactionId)
	assert.Equal(t, base58.Encode(transactionId), *fulfillmentRecord.Signature)

	if int(txn.Message.Header.NumSignatures) > 1 {
		assert.True(t, ed25519.Verify(txn.Message.Accounts[1], txn.Message.Marshal(), txn.Signatures[1][:]))
	}
}

func (s serverTestEnv) assertOnDemandTransaction(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	assert.Nil(t, fulfillmentRecord.Signature)
	assert.Nil(t, fulfillmentRecord.Nonce)
	assert.Nil(t, fulfillmentRecord.Blockhash)
	assert.Empty(t, fulfillmentRecord.Data)
}

func (s serverTestEnv) assertExpectedCloseEmptyTimelockAccountTransaction(t *testing.T, intentType intent.Type, fulfillmentRecord *fulfillment.Record, authority *common.Account) {
	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

	require.Len(t, txn.Message.Instructions, 3)

	_, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	dataVersion := timelock_token_v1.DataVersion1
	if intentType == intent.MigrateToPrivacy2022 {
		dataVersion = timelock_token_v1.DataVersionLegacy
	}

	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	require.NoError(t, err)

	if dataVersion == timelock_token_v1.DataVersion1 {
		burnIxnArgs, burnIxnAccounts, err := timelock_token_v1.BurnDustWithAuthorityInstructionFromLegacyInstruction(txn, 1)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, burnIxnArgs.TimelockBump)
		assert.Equal(t, kin.ToQuarks(1), burnIxnArgs.MaxAmount)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), burnIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), burnIxnAccounts.Vault)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), burnIxnAccounts.VaultOwner)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), burnIxnAccounts.TimeAuthority)
		assert.EqualValues(t, kin.TokenMint, burnIxnAccounts.Mint)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), burnIxnAccounts.Payer)

		closeIxnArgs, closeIxnAccounts, err := timelock_token_v1.CloseAccountsInstructionFromLegacyInstruction(txn, 2)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, closeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), closeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), closeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.CloseAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.Payer)
	} else {
		burnIxnArgs, burnIxnAccounts, err := timelock_token_legacy.BurnDustWithAuthorityInstructionFromLegacyInstruction(txn, 1)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, burnIxnArgs.TimelockBump)
		assert.Equal(t, kin.ToQuarks(1), burnIxnArgs.MaxAmount)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), burnIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), burnIxnAccounts.Vault)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), burnIxnAccounts.VaultOwner)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), burnIxnAccounts.TimeAuthority)
		assert.EqualValues(t, kin.TokenMint, burnIxnAccounts.Mint)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), burnIxnAccounts.Payer)

		closeIxnArgs, closeIxnAccounts, err := timelock_token_legacy.CloseAccountsInstructionFromLegacyInstruction(txn, 2)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, closeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), closeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), closeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.CloseAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.Payer)
	}
}

func (s serverTestEnv) assertExpectedCloseTimelockAccountWithBalanceTransaction(t *testing.T, intentType intent.Type, fulfillmentRecord *fulfillment.Record, authority, destination *common.Account, expectedAdditionalMemo *string) {
	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

	expectedInstructionCount := 6
	if expectedAdditionalMemo != nil {
		expectedInstructionCount = 7
	}
	require.Len(t, txn.Message.Instructions, expectedInstructionCount)

	nextIxnIndex := 0

	_, err := system.DecompileAdvanceNonce(txn.Message, nextIxnIndex)
	require.NoError(t, err)
	nextIxnIndex++

	assertExpectedKreMemoInstruction(t, txn, nextIxnIndex)
	nextIxnIndex++

	if expectedAdditionalMemo != nil {
		memo, err := memo.DecompileMemo(txn.Message, nextIxnIndex)
		require.NoError(t, err)
		assert.Equal(t, *expectedAdditionalMemo, string(memo.Data))
		nextIxnIndex++
	}

	dataVersion := timelock_token_v1.DataVersion1
	if intentType == intent.MigrateToPrivacy2022 {
		dataVersion = timelock_token_v1.DataVersionLegacy
	}

	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	require.NoError(t, err)

	if dataVersion == timelock_token_v1.DataVersion1 {
		revokeIxnArgs, revokeIxnAccounts, err := timelock_token_v1.RevokeLockWithAuthorityFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, revokeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), revokeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), revokeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), revokeIxnAccounts.TimeAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), revokeIxnAccounts.Payer)

		nextIxnIndex++

		deactivateIxnArgs, deactivateIxnAccounts, err := timelock_token_v1.DeactivateInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, deactivateIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), deactivateIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), deactivateIxnAccounts.VaultOwner)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), deactivateIxnAccounts.Payer)

		nextIxnIndex++

		withdrawIxnArgs, withdrawIxnAccounts, err := timelock_token_v1.WithdrawInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, withdrawIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), withdrawIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), withdrawIxnAccounts.Vault)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), withdrawIxnAccounts.VaultOwner)
		assert.EqualValues(t, destination.PublicKey().ToBytes(), withdrawIxnAccounts.Destination)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), withdrawIxnAccounts.Payer)

		nextIxnIndex++

		closeIxnArgs, closeIxnAccounts, err := timelock_token_v1.CloseAccountsInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, closeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), closeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), closeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.CloseAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.Payer)
	} else {
		revokeIxnArgs, revokeIxnAccounts, err := timelock_token_legacy.RevokeLockWithAuthorityFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, revokeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), revokeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), revokeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), revokeIxnAccounts.TimeAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), revokeIxnAccounts.Payer)

		nextIxnIndex++

		deactivateIxnArgs, deactivateIxnAccounts, err := timelock_token_legacy.DeactivateInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, deactivateIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), deactivateIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), deactivateIxnAccounts.VaultOwner)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), deactivateIxnAccounts.Payer)

		nextIxnIndex++

		withdrawIxnArgs, withdrawIxnAccounts, err := timelock_token_legacy.WithdrawInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, withdrawIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), withdrawIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), withdrawIxnAccounts.Vault)
		assert.EqualValues(t, timelockAccounts.VaultOwner.PublicKey().ToBytes(), withdrawIxnAccounts.VaultOwner)
		assert.EqualValues(t, destination.PublicKey().ToBytes(), withdrawIxnAccounts.Destination)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), withdrawIxnAccounts.Payer)

		nextIxnIndex++

		closeIxnArgs, closeIxnAccounts, err := timelock_token_legacy.CloseAccountsInstructionFromLegacyInstruction(txn, nextIxnIndex)
		require.NoError(t, err)

		assert.Equal(t, timelockAccounts.StateBump, closeIxnArgs.TimelockBump)

		assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), closeIxnAccounts.Timelock)
		assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), closeIxnAccounts.Vault)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.CloseAuthority)
		assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), closeIxnAccounts.Payer)
	}
}

func (s serverTestEnv) assertExpectedTransferWithAuthorityTransaction(t *testing.T, fulfillmentRecord *fulfillment.Record, authority, destination *common.Account, amount uint64) {
	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(fulfillmentRecord.Data))

	require.Len(t, txn.Message.Instructions, 3)

	_, err := system.DecompileAdvanceNonce(txn.Message, 0)
	require.NoError(t, err)

	assertExpectedKreMemoInstruction(t, txn, 1)

	sourceTimelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	transferIxnArgs, transferIxnAccounts, err := timelock_token_v1.TransferWithAuthorityInstructionFromLegacyInstruction(txn, 2)
	require.NoError(t, err)

	assert.Equal(t, amount, transferIxnArgs.Amount)
	assert.Equal(t, sourceTimelockAccounts.StateBump, transferIxnArgs.TimelockBump)

	assert.EqualValues(t, sourceTimelockAccounts.State.PublicKey().ToBytes(), transferIxnAccounts.Timelock)
	assert.EqualValues(t, sourceTimelockAccounts.Vault.PublicKey().ToBytes(), transferIxnAccounts.Vault)
	assert.EqualValues(t, sourceTimelockAccounts.VaultOwner.PublicKey().ToBytes(), transferIxnAccounts.VaultOwner)
	assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), transferIxnAccounts.TimeAuthority)
	assert.EqualValues(t, destination.PublicKey().ToBytes(), transferIxnAccounts.Destination)
	assert.EqualValues(t, s.subsidizer.PublicKey().ToBytes(), transferIxnAccounts.Payer)
}

func (s serverTestEnv) getExpectedTreasury(t *testing.T, amount uint64) *treasury.Record {
	bucket := uint64(math.Pow(10, float64(len(strconv.Itoa(int(amount))))-1))
	treasury, ok := s.treasuryPoolByBucket[bucket]
	require.True(t, ok)
	return treasury
}

func (s serverTestEnv) assertFiatOnrampPurchasedDetailsSaved(t *testing.T, expectedOwner *common.Account, expectedCurrency currency_lib.Code, expectedAmount float64, nonce uuid.UUID) {
	record, err := s.data.GetFiatOnrampPurchase(s.ctx, nonce)
	require.NoError(t, err)
	assert.Equal(t, expectedOwner.PublicKey().ToBase58(), record.Owner)
	assert.EqualValues(t, expectedCurrency, record.Currency)
	assert.Equal(t, expectedAmount, record.Amount)
	assert.Equal(t, nonce, record.Nonce)
}

func (s serverTestEnv) assertFiatOnrampPurchasedDetailsNotSaved(t *testing.T, nonce uuid.UUID) {
	_, err := s.data.GetFiatOnrampPurchase(s.ctx, nonce)
	assert.Equal(t, onramp.ErrPurchaseNotFound, err)
}

func assertExpectedKreMemoInstruction(t *testing.T, txn solana.Transaction, index int) {
	memo, err := memo.DecompileMemo(txn.Message, index)
	require.NoError(t, err)

	kreMemo, err := kin.MemoFromBase64String(string(memo.Data), true)
	require.NoError(t, err)
	assert.EqualValues(t, kin.HighestVersion, kreMemo.Version())
	assert.EqualValues(t, kin.TransactionTypeP2P, kreMemo.TransactionType())
	assert.EqualValues(t, transaction_util.KreAppIndex, kreMemo.AppIndex())
}

type derivedAccount struct {
	accountType    commonpb.AccountType
	index          uint64
	relationshipTo *string
	value          *common.Account
}

type txnToSign struct {
	txn       solana.Transaction
	authority *common.Account
}

type privacyUpgradeMetadata struct {
	intentId string
	actionId uint32

	source              *common.TimelockAccounts
	treasuryPool        *common.Account
	recentRoot          []byte
	originalCommitment  *common.Account
	originalDestination *common.Account
	amount              uint64

	originalTransactionBlob []byte

	nonce *common.Account
	bh    solana.Blockhash
}

// Note: Most of these flags are code for one being set at a time, which is their
// intended use to test specific failure scenarios.
type phoneConf struct {
	//
	// Simulations for authentication and authorization checks
	//

	simulateInvalidSubmitIntentRequestSignature                    bool
	simulateInvalidOpenAccountOwner                                bool
	simulateInvalidOpenAccountSignature                            bool
	simulateInvalidGetPrioritizedIntentsForPrivacyUpgradeSignature bool

	//
	// Simulations for transaction signature submission
	//

	simulateTooManySubmittedSignatures     bool
	simulateTooFewSubmittedSignatures      bool
	simulateInvalidSignatureValueSubmitted bool

	//
	// Simulations for device verification
	//
	simulateNoDeviceTokenProvided      bool
	simualteInvalidDeviceTokenProvided bool

	//
	// Simulations for OpenAccounts intent validation
	//

	simulateOpeningTooManyAccounts bool
	simulateOpeningTooFewAccounts  bool

	//
	// Simulations for SendPrivatePayment and SendPublicPayment intent validation
	//

	simulateSendingTooLittle        bool
	simulateSendingTooMuch          bool
	simulateNotSendingFromSource    bool
	simulateNotSendingToDestination bool

	simulateSendPrivatePaymentFromPreviousTempOutgoingAccount       bool
	simulateSendPrivatePaymentFromTempIncomingAccount               bool
	simulateSendPrivatePaymentFromBucketAccount                     bool
	simulateSendPrivatePaymentWithMoreTempOutgoingOutboundTransfers bool
	simulateSendPrivatePaymentWithoutClosingTempIncomingAccount     bool

	simulateSendPublicPaymentPrivately         bool
	simulateSendPublicPaymentWithWithdraw      bool
	simulateSendPublicPaymentFromBucketAccount bool

	simulateNotOpeningGiftCardAccount                bool
	simulateOpeningWrongGiftCardAccount              bool
	simulateOpeningGiftCardWithWrongOwner            bool
	simulateOpeningGiftCardWithUser12Words           bool
	simulateOpeningGiftCardAsWrongAccountType        bool
	simulateClosingGiftCardAccount                   bool
	simulateNotClosingDormantGiftCardAccount         bool
	simulateClosingDormantGiftCardAsWrongAccountType bool
	simulateInvalidGiftCardIndex                     bool

	simulateFlippingWithdrawFlag bool

	simulateInvalidExchangeRate bool
	simulateInvalidNativeAmount bool

	//
	// Simulations for ReceivePaymentsPrivately and ReceivePaymentsPublicly intent validation
	//

	simulateReceivingFromDesktop bool

	simulateReceivingTooLittle bool
	simulateReceivingTooMuch   bool

	simulateClaimingTooLittleFromGiftCard bool
	simulateClaimingTooMuchFromGiftCard   bool

	simulateNotReceivingFromSource                        bool
	simulateReceivePaymentFromPreviousTempIncomingAccount bool
	simulateReceivePaymentFromTempOutgoingAccount         bool
	simulateReceivePaymentFromBucketAccount               bool

	simulateFlippingDepositFlag bool

	simulateReceivePaymentsPubliclyWithoutWithdraw bool
	simulateReceivePaymentsPubliclyPrivately       bool

	//
	// Simulations for all payment intent validation
	//

	simulateUsingPrimaryAccountAsSource      bool
	simulateUsingPrimaryAccountAsDestination bool

	simulateFundingTempAccountTooMuch                    bool
	simulateOpeningWrongTempAccount                      bool
	simulateUsingNewTempAccount                          bool
	simulateUsingCurrentTempIncomingAccountAsSource      bool
	simulateUsingCurrentTempIncomingAccountAsDestination bool
	simulateUsingCurrentTempOutgoingAccountAsDestination bool
	simulateNotClosingTempAccount                        bool
	simulateClosingWrongAccount                          bool

	simulateInvalidBucketExchangeMultiple   bool
	simulateInvalidAnyonymizedBucketAmount  bool
	simulateBucketExchangeInATransfer       bool
	simulateBucketExchangeInAPublicTransfer bool
	simulateBucketExchangeInAPublicWithdraw bool

	simulateSwappingTransfersForExchanges bool
	simulateSwappingExchangesForTransfers bool

	simulateTooManyTemporaryPrivacyTransfers bool
	simulateTooManyTemporaryPrivacyExchanges bool

	simulateFlippingRemoteSendFlag bool
	simulateUsingGiftCardAccount   bool

	simulateFlippingTipFlag bool

	//
	// Simulations for EstablishRelationship intent validation
	//

	simulateDerivingDifferentRelationshipAccount bool

	//
	// Simulations for UpgradePrivacy intent validation
	//

	simulateDoublePrivacyUpgradeInSameRequest bool
	simulateInvalidActionDuringPrivacyUpgrade bool

	//
	// Simulations for MigrateToPrivacy2022 intent validation
	//

	simulateMigratingFundsToWrongAccount       bool
	simulateMigratingUsingOppositeAction       bool
	simulateMigratingUsingWrongAction          bool
	simulatingMigratingWithWrongAmountInAction bool

	//
	// Simulations for general action valiation across all intents
	//

	simulateInvalidActionId bool

	simulateInvalidAccountTypeForOpenPrimaryAccountAction  bool
	simulateInvalidAuthorityForOpenPrimaryAccountAction    bool
	simulateInvalidTokenAccountForOpenPrimaryAccountAction bool
	simulateInvalidIndexForOpenPrimaryAccountAction        bool
	simulateOpenPrimaryAccountActionReplaced               bool

	simulateInvalidAccountTypeForOpenNonPrimaryAccountAction  bool
	simulateInvalidAuthorityForOpenNonPrimaryAccountAction    bool
	simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction    bool
	simulateInvalidTokenAccountForOpenNonPrimaryAccountAction bool
	simulateInvalidIndexForOpenNonPrimaryAccountAction        bool
	simulateOpenNonPrimaryAccountActionReplaced               bool

	simulateInvalidAccountTypeForCloseDormantAccountAction   bool
	simulateInvalidAuthorityForCloseDormantAccountAction     bool
	simulateInvalidTokenAccountForCloseDormantAccountAction  bool
	simulateInvalidDestitinationForCloseDormantAccountAction bool
	simulateCloseDormantAccountActionReplaced                bool

	simulateInvalidAuthorityForCloseEmptyAccountAction    bool
	simulateInvalidTokenAccountForCloseEmptyAccountAction bool

	simulateInvalidTokenAccountForNoPrivacyTransferAction bool
	simulateInvalidTokenAccountForNoPrivacyWithdrawAction bool

	simulateInvalidTokenAccountForTemporaryPrivacyTransferAction bool

	simulateInvalidTokenAccountForTemporaryPrivacyExchangeAction bool

	simulateRandomOpenAccountActionInjected         bool
	simulateRandomCloseEmptyAccountActionInjected   bool
	simulateRandomCloseDormantAccountActionInjected bool
	simulatePrivacyUpgradeActionInjected            bool

	//
	// Simulations for slow clients
	//

	simulateDelayForSubmittingActions    bool
	simulateDelayForSubmittingSignatures bool

	//
	//	Simulations for improper use of intent IDs
	//

	simulateReusingIntentId              bool
	simulateUpgradeToNonExistantIntentId bool

	//
	// Simulation for requests
	//
	simulatePaymentRequest                        bool
	simulateLoginRequest                          bool
	simulateUnverifiedPaymentRequest              bool
	simulateInvalidPaymentRequestDestination      bool
	simulateInvalidPaymentRequestExchangeCurrency bool
	simulateInvalidPaymentRequestNativeAmount     bool
	simulateAdditionalFees                        bool
	simulateCodeFeePaid                           bool
	simulateCodeFeeNotPaid                        bool
	simulateSmallCodeFee                          bool
	simulateLargeCodeFee                          bool
	simulateCodeFeeAsThirdParyFee                 bool
	simulateMultipleCodeFeePayments               bool
	simulateTooManyThirdPartyFees                 bool
	simulateThirdPartyFeeMissing                  bool
	simulateInvalidThirdPartyFeeAmount            bool
	simulateInvalidThirdPartyFeeDestination       bool
	simulateThirdPartyFeeDestinationMissing       bool
	simulateThirdParyFeeAsCodeFee                 bool
}

type phoneTestEnv struct {
	ctx                      context.Context
	client                   transactionpb.TransactionClient
	conf                     phoneConf
	parentAccount            *common.Account
	currentDerivedAccounts   map[commonpb.AccountType]*common.Account
	currentTempIncomingIndex uint64
	currentTempOutgoingIndex uint64
	allGiftCardAccounts      []*common.Account
	allDerivedAccounts       []derivedAccount
	txnsToUpgrade            []privacyUpgradeMetadata
	verifiedPhoneNumber      string
	lastIntentSubmitted      string
	directServerAccess       *serverTestEnv // Purely as a convenience to setup invalid intents
}

type submitIntentCallMetadata struct {
	intentId      string
	rendezvousKey *common.Account
	protoMetadata *transactionpb.Metadata
	protoActions  []*transactionpb.Action
	resp          *transactionpb.SubmitIntentResponse
	err           error
}

func (p *phoneTestEnv) openAccounts(t *testing.T) submitIntentCallMetadata {
	var actions []*transactionpb.Action
	for _, accountType := range accountTypesToOpen {
		var index uint64
		if accountType == commonpb.AccountType_TEMPORARY_INCOMING {
			index = p.currentTempIncomingIndex
		} else if accountType == commonpb.AccountType_TEMPORARY_OUTGOING {
			index = p.currentTempOutgoingIndex
		}

		authority := p.currentDerivedAccounts[accountType]
		if accountType == commonpb.AccountType_PRIMARY {
			authority = p.parentAccount
		}

		openAccountAction := &transactionpb.Action{
			Type: &transactionpb.Action_OpenAccount{
				OpenAccount: &transactionpb.OpenAccountAction{
					AccountType: accountType,
					Owner:       p.parentAccount.ToProto(),
					Authority:   authority.ToProto(),
					Token:       p.getTimelockVault(t, accountType, 0).ToProto(),
					Index:       index,
				},
			},
		}
		actions = append(actions, openAccountAction)

		if accountType != commonpb.AccountType_PRIMARY {
			closeAccountAction := &transactionpb.Action{
				Type: &transactionpb.Action_CloseDormantAccount{
					CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
						AccountType: accountType,
						Authority:   authority.ToProto(),
						Token:       p.getTimelockVault(t, accountType, 0).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
					},
				},
			}
			actions = append(actions, closeAccountAction)
		}
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_OpenAccounts{},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) send42KinToGiftCardAccount(t *testing.T, giftCardAccount *common.Account) submitIntentCallMetadata {

	// Generate a new random gift card account (no derivation logic, index, etc...)
	p.allGiftCardAccounts = append(p.allGiftCardAccounts, giftCardAccount)

	// Generate a new outgoing temp account
	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// --------------------------------------------------------------------
		// Section 1: Open REMOTE_SEND_GIFT_CARD account
		// --------------------------------------------------------------------
		{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
				Owner:       giftCardAccount.ToProto(),
				Authority:   giftCardAccount.ToProto(),
				Token:       getTimelockVault(t, giftCardAccount).ToProto(),
				Index:       0,
			},
		}},

		// --------------------------------------------------------------------
		// Section 2: Transfer ExchangeData.Quarks from BUCKET_X_KIN accounts to
		// 			  TEMPORARY_OUTGOING account with reogranizations
		// --------------------------------------------------------------------
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(2),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(40),
			},
		}},

		// Re-organize bucket accounts (example)
		{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// --------------------------------------------------------------------
		// Section 3: Rotate TEMPORARY_OUTGOING account
		// --------------------------------------------------------------------

		// Send full payment from temporary outgoing account to the gift card account
		{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: getTimelockVault(t, giftCardAccount).ToProto(),
				Amount:      kin.ToQuarks(42),
				ShouldClose: true,
			},
		}},

		// Rotate to new temporary outgoing account
		{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},

		// --------------------------------------------------------------------
		// Section 4: Close REMOTE_SEND_GIFT_CARD if not redeemed after period of time
		// --------------------------------------------------------------------
		{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
				Authority:   giftCardAccount.ToProto(),
				Token:       getTimelockVault(t, giftCardAccount).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				IsRemoteSend: true,
				IsWithdrawal: false,
				Destination:  getTimelockVault(t, giftCardAccount).ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "cad",
					ExchangeRate: 0.05,
					NativeAmount: 2.1,
					Quarks:       kin.ToQuarks(42),
				},
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) send42KinToCodeUser(t *testing.T, receiver phoneTestEnv) submitIntentCallMetadata {
	destination := receiver.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, receiver.currentTempIncomingIndex)

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(2),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(40),
			},
		}},

		// Send full payment from temporary outgoing account to payment destination
		{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(42),
				ShouldClose: true,
			},
		}},

		// Re-organize bucket accounts (example)
		{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate to new temporary outgoing account
		{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 4.2,
					Quarks:       kin.ToQuarks(42),
				},
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) receive42KinPrivatelyIntoOrganizer(t *testing.T) submitIntentCallMetadata {
	nextIndex := p.currentTempIncomingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_INCOMING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Receive from temporary incoming to bucketed accounts
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_INCOMING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(2),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_INCOMING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(40),
			},
		}},

		// Re-organize bucket accounts (example)
		{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate temporary incoming account
		{Type: &transactionpb.Action_CloseEmptyAccount{
			CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_INCOMING,
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_INCOMING].ToProto(),
				Token:       p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
			},
		}},
		{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_INCOMING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_INCOMING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_ReceivePaymentsPrivately{
			ReceivePaymentsPrivately: &transactionpb.ReceivePaymentsPrivatelyMetadata{
				Source: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
				Quarks: kin.ToQuarks(42),
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempIncomingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_INCOMING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) receive42KinFromCodeUser(t *testing.T) submitIntentCallMetadata {
	return p.receive42KinPrivatelyIntoOrganizer(t)
}

func (p *phoneTestEnv) receive42KinFromGiftCard(t *testing.T, giftCardAccount *common.Account, isVoided bool) submitIntentCallMetadata {
	p.allGiftCardAccounts = append(p.allGiftCardAccounts, giftCardAccount)

	actions := []*transactionpb.Action{
		// Receive full amount from gift card to latest temp incoming in a single withdrawal
		{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   giftCardAccount.ToProto(),
				Source:      getTimelockVault(t, giftCardAccount).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
				Amount:      kin.ToQuarks(42),
				ShouldClose: true,
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_ReceivePaymentsPublicly{
			ReceivePaymentsPublicly: &transactionpb.ReceivePaymentsPubliclyMetadata{
				Source:                  getTimelockVault(t, giftCardAccount).ToProto(),
				Quarks:                  kin.ToQuarks(42),
				IsRemoteSend:            true,
				IsIssuerVoidingGiftCard: isVoided,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) publiclyWithdraw123KinToExternalWallet(t *testing.T) submitIntentCallMetadata {
	destination := testutil.NewRandomAccount(t)

	actions := []*transactionpb.Action{
		// Send full payment from primary to payment destination in a single transfer
		{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(123),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPublicPayment{
			SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "kin",
					ExchangeRate: 1,
					NativeAmount: 123,
					Quarks:       kin.ToQuarks(123),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) publiclyWithdraw123KinToExternalWalletFromRelationshipAccount(t *testing.T, relationship string) submitIntentCallMetadata {
	destination := testutil.NewRandomAccount(t)

	sourceAuthority := p.getAuthorityForRelationshipAccount(t, relationship)

	actions := []*transactionpb.Action{
		// Send full payment from primary to payment destination in a single transfer
		{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(123),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPublicPayment{
			SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "kin",
					ExchangeRate: 1,
					NativeAmount: 123,
					Quarks:       kin.ToQuarks(123),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) privatelyWithdraw123KinToExternalWallet(t *testing.T) submitIntentCallMetadata {
	totalAmount := kin.ToQuarks(123)

	destination := testutil.NewRandomAccount(t)

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(3),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(20),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_100_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(100),
			},
		}},
	}

	var feePayments uint64
	if p.conf.simulatePaymentRequest {
		// Pay mandatory hard-coded Code $0.01 USD fee
		codeFeePayment := kin.ToQuarks(1) / 10 // 0.1 Kin
		feePayments += codeFeePayment
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_FeePayment{
				FeePayment: &transactionpb.FeePaymentAction{
					Type:      transactionpb.FeePaymentAction_CODE,
					Authority: p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
					Source:    p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
					Amount:    codeFeePayment,
				},
			},
		})

		// Pay additional fees as configured by the third party
		if p.conf.simulateAdditionalFees {
			for i := 0; i < 3; i++ {
				requestedFee := (uint64(defaultTestThirdPartyFeeBps) * totalAmount) / 10000
				feePayments += requestedFee
				actions = append(actions, &transactionpb.Action{
					// Pay any fees when applicable
					Type: &transactionpb.Action_FeePayment{
						FeePayment: &transactionpb.FeePaymentAction{
							Type:        transactionpb.FeePaymentAction_THIRD_PARTY,
							Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
							Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
							Destination: testutil.NewRandomAccount(t).ToProto(),
							Amount:      requestedFee,
						},
					},
				})
			}
		}
	}

	actions = append(
		actions,
		// Send full payment from temporary outgoing account to payment destination,
		// minus any fees
		&transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(123) - feePayments,
				ShouldClose: true,
			},
		}},

		// Re-organize bucket accounts (example)
		&transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate to new temporary outgoing account
		&transactionpb.Action{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		&transactionpb.Action{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	)

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "kin",
					ExchangeRate: 1,
					NativeAmount: 123,
					Quarks:       kin.ToQuarks(123),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) privatelyWithdrawMillionDollarsToExternalWallet(t *testing.T) submitIntentCallMetadata {
	destination := testutil.NewRandomAccount(t)

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_000_000_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_000_000_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(10_000_000),
			},
		}},

		// Send full payment from temporary outgoing account to payment destination
		{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(10_000_000),
				ShouldClose: true,
			},
		}},

		// Rotate to new temporary outgoing account
		{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 1_000_000,
					Quarks:       kin.ToQuarks(10_000_000),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) depositMillionDollarsIntoOrganizer(t *testing.T) submitIntentCallMetadata {
	actions := []*transactionpb.Action{
		// Receive from primary account to bucketed accounts
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_000_000_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10_000_000),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_ReceivePaymentsPrivately{
			ReceivePaymentsPrivately: &transactionpb.ReceivePaymentsPrivatelyMetadata{
				Source:    p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Quarks:    kin.ToQuarks(10_000_000),
				IsDeposit: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) deposit777KinIntoOrganizer(t *testing.T) submitIntentCallMetadata {
	actions := []*transactionpb.Action{
		// Receive from temporary incoming to bucketed accounts
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(7),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(70),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(700),
			},
		}},

		// Re-organize bucket accounts (example)
		{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_ReceivePaymentsPrivately{
			ReceivePaymentsPrivately: &transactionpb.ReceivePaymentsPrivatelyMetadata{
				Source:    p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Quarks:    kin.ToQuarks(777),
				IsDeposit: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) deposit777KinIntoOrganizerFromRelationshipAccount(t *testing.T, relationship string) submitIntentCallMetadata {
	sourceAuthority := p.getAuthorityForRelationshipAccount(t, relationship)

	actions := []*transactionpb.Action{
		// Receive from temporary incoming to bucketed accounts
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(7),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(70),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(700),
			},
		}},

		// Re-organize bucket accounts (example)
		{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_ReceivePaymentsPrivately{
			ReceivePaymentsPrivately: &transactionpb.ReceivePaymentsPrivatelyMetadata{
				Source:    getTimelockVault(t, sourceAuthority).ToProto(),
				Quarks:    kin.ToQuarks(777),
				IsDeposit: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t *testing.T, receiver phoneTestEnv) submitIntentCallMetadata {
	destination := receiver.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0)

	actions := []*transactionpb.Action{
		// Send full payment from primary to payment destination in a single transfer
		{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: destination.ToProto(),
				Amount:      kin.ToQuarks(777),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPublicPayment{
			SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 77.7,
					Quarks:       kin.ToQuarks(777),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) publiclyWithdraw777KinToCodeFromRelationshipToPrimaryAccount(t *testing.T, relationship string, receiver phoneTestEnv) submitIntentCallMetadata {
	sourceAuthority := p.getAuthorityForRelationshipAccount(t, relationship)
	source := getTimelockVault(t, sourceAuthority)

	actions := []*transactionpb.Action{
		// Send full payment from relationship to payment destination in a single transfer
		{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      source.ToProto(),
				Destination: receiver.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Amount:      kin.ToQuarks(777),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPublicPayment{
			SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
				Source:      source.ToProto(),
				Destination: receiver.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 77.7,
					Quarks:       kin.ToQuarks(777),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) publiclyWithdraw777KinToCodeUserBetweenRelationshipAccounts(t *testing.T, relationship string, receiver phoneTestEnv) submitIntentCallMetadata {
	sourceAuthority := p.getAuthorityForRelationshipAccount(t, relationship)
	destinationAuthority := receiver.getAuthorityForRelationshipAccount(t, relationship)

	actions := []*transactionpb.Action{
		// Send full payment from primary to payment destination in a single transfer
		{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   sourceAuthority.ToProto(),
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: getTimelockVault(t, destinationAuthority).ToProto(),
				Amount:      kin.ToQuarks(777),
			},
		}},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPublicPayment{
			SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
				Source:      getTimelockVault(t, sourceAuthority).ToProto(),
				Destination: getTimelockVault(t, destinationAuthority).ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 77.7,
					Quarks:       kin.ToQuarks(777),
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) privatelyWithdraw777KinToCodeUser(t *testing.T, receiver phoneTestEnv) submitIntentCallMetadata {
	totalAmount := kin.ToQuarks(777)

	destination := receiver.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0)

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(7),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(70),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_100_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(700),
			},
		}},
	}

	var feePayments uint64
	if p.conf.simulatePaymentRequest {
		// Pay mandatory hard-coded Code $0.01 USD fee
		codeFeePayment := kin.ToQuarks(1) / 10 // 0.1 Kin
		feePayments += codeFeePayment
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_FeePayment{
				FeePayment: &transactionpb.FeePaymentAction{
					Type:      transactionpb.FeePaymentAction_CODE,
					Authority: p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
					Source:    p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
					Amount:    codeFeePayment,
				},
			},
		})

		// Pay additional fees as configured by the third party
		if p.conf.simulateAdditionalFees {
			for i := 0; i < 3; i++ {
				requestedFee := (uint64(defaultTestThirdPartyFeeBps) * totalAmount) / 10000
				feePayments += requestedFee
				actions = append(actions, &transactionpb.Action{
					// Pay any fees when applicable
					Type: &transactionpb.Action_FeePayment{
						FeePayment: &transactionpb.FeePaymentAction{
							Type:        transactionpb.FeePaymentAction_THIRD_PARTY,
							Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
							Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
							Destination: testutil.NewRandomAccount(t).ToProto(),
							Amount:      requestedFee,
						},
					},
				})
			}
		}
	}

	actions = append(
		actions,
		// Send full payment from temporary outgoing account to payment destination,
		// minus any fees
		&transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      totalAmount - feePayments,
				ShouldClose: true,
			},
		}},

		// Re-organize bucket accounts (example)
		&transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate to new temporary outgoing account
		&transactionpb.Action{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		&transactionpb.Action{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	)

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 77.7,
					Quarks:       totalAmount,
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) privatelyWithdraw321KinToCodeUserRelationshipAccount(t *testing.T, receiver phoneTestEnv, relationship string) submitIntentCallMetadata {
	totalAmount := kin.ToQuarks(321)

	destination := getTimelockVault(t, receiver.getAuthorityForRelationshipAccount(t, relationship))

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(1),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(20),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_100_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(300),
			},
		}},
	}

	var feePayments uint64
	if p.conf.simulatePaymentRequest {
		// Pay mandatory hard-coded Code $0.01 USD fee
		codeFeePayment := kin.ToQuarks(1) / 10 // 0.1 Kin
		feePayments += codeFeePayment
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_FeePayment{
				FeePayment: &transactionpb.FeePaymentAction{
					Type:      transactionpb.FeePaymentAction_CODE,
					Authority: p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
					Source:    p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
					Amount:    codeFeePayment,
				},
			},
		})

		// Pay additional fees as configured by the third party
		if p.conf.simulateAdditionalFees {
			for i := 0; i < 3; i++ {
				requestedFee := (uint64(defaultTestThirdPartyFeeBps) * totalAmount) / 10000
				feePayments += requestedFee
				actions = append(actions, &transactionpb.Action{
					// Pay any fees when applicable
					Type: &transactionpb.Action_FeePayment{
						FeePayment: &transactionpb.FeePaymentAction{
							Type:        transactionpb.FeePaymentAction_THIRD_PARTY,
							Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
							Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
							Destination: testutil.NewRandomAccount(t).ToProto(),
							Amount:      requestedFee,
						},
					},
				})
			}
		}
	}

	actions = append(
		actions,
		// Send full payment from temporary outgoing account to payment destination,
		// minus any fees
		&transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      totalAmount - feePayments,
				ShouldClose: true,
			},
		}},

		// Re-organize bucket accounts (example)
		&transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate to new temporary outgoing account
		&transactionpb.Action{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		&transactionpb.Action{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	)

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 32.1,
					Quarks:       totalAmount,
				},
				IsWithdrawal: true,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) tip456KinToCodeUser(t *testing.T, receiver phoneTestEnv, username string) submitIntentCallMetadata {
	totalAmount := kin.ToQuarks(456)

	destination := receiver.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0)

	nextIndex := p.currentTempOutgoingIndex + 1
	nextDerivedAccount := derivedAccount{
		accountType: commonpb.AccountType_TEMPORARY_OUTGOING,
		index:       nextIndex,
		value:       testutil.NewRandomAccount(t),
	}
	p.allDerivedAccounts = append(p.allDerivedAccounts, nextDerivedAccount)

	actions := []*transactionpb.Action{
		// Send bucketed amounts to temporary outgoing account
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(6),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(50),
			},
		}},
		{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_100_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(400),
			},
		}},
	}

	actions = append(
		actions,
		// Send full payment from temporary outgoing account to payment destination,
		// minus any fees
		&transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Destination: destination.ToProto(),
				Amount:      totalAmount,
				ShouldClose: true,
			},
		}},

		// Re-organize bucket accounts (example)
		&transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10),
			},
		}},

		// Rotate to new temporary outgoing account
		&transactionpb.Action{Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Owner:       p.parentAccount.ToProto(),
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Index:       nextIndex,
			},
		}},
		&transactionpb.Action{Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				AccountType: commonpb.AccountType_TEMPORARY_OUTGOING,
				Authority:   nextDerivedAccount.value.ToProto(),
				Token:       getTimelockVault(t, nextDerivedAccount.value).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
			},
		}},
	)

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_SendPrivatePayment{
			SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
				Destination: destination.ToProto(),
				ExchangeData: &transactionpb.ExchangeData{
					Currency:     "usd",
					ExchangeRate: 0.1,
					NativeAmount: 45.6,
					Quarks:       totalAmount,
				},
				IsWithdrawal: true,
				IsTip:        true,
				TippedUser: &transactionpb.TippedUser{
					Platform: transactionpb.TippedUser_TWITTER,
					Username: username,
				},
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.currentTempOutgoingIndex = nextIndex
		p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING] = nextDerivedAccount.value
	}
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) migrateToPrivacy2022(t *testing.T, quarks uint64) submitIntentCallMetadata {
	actions := []*transactionpb.Action{}

	legacyTimelockAccounts, err := p.parentAccount.GetTimelockAccounts(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
	require.NoError(t, err)

	if quarks == 0 {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_CloseEmptyAccount{
			CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
				AccountType: commonpb.AccountType_LEGACY_PRIMARY_2022,
				Authority:   p.parentAccount.ToProto(),
				Token:       legacyTimelockAccounts.Vault.ToProto(),
			},
		}})
	} else {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      legacyTimelockAccounts.Vault.ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Amount:      quarks,
				ShouldClose: true,
			},
		}})
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_MigrateToPrivacy_2022{
			MigrateToPrivacy_2022: &transactionpb.MigrateToPrivacy2022Metadata{
				Quarks: quarks,
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) establishRelationshipWithMerchant(t *testing.T, domain string) submitIntentCallMetadata {
	authority := testutil.NewRandomAccount(t)
	foundExistingAuthority := false
	for _, derivedAccount := range p.allDerivedAccounts {
		if derivedAccount.accountType == commonpb.AccountType_RELATIONSHIP && *derivedAccount.relationshipTo == domain {
			foundExistingAuthority = true
			authority = derivedAccount.value
			break
		}
	}

	if !foundExistingAuthority {
		p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{
			accountType:    commonpb.AccountType_RELATIONSHIP,
			index:          0,
			relationshipTo: &domain,
			value:          authority,
		})
	}

	actions := []*transactionpb.Action{
		{
			Type: &transactionpb.Action_OpenAccount{
				OpenAccount: &transactionpb.OpenAccountAction{
					AccountType: commonpb.AccountType_RELATIONSHIP,
					Owner:       p.parentAccount.ToProto(),
					Authority:   authority.ToProto(),
					Token:       getTimelockVault(t, authority).ToProto(),
					Index:       0,
				},
			},
		},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_EstablishRelationship{
			EstablishRelationship: &transactionpb.EstablishRelationshipMetadata{
				Relationship: &commonpb.Relationship{
					Type: &commonpb.Relationship_Domain{
						Domain: &commonpb.Domain{
							Value: domain,
						},
					},
				},
			},
		},
	}

	rendezvousKey := testutil.NewRandomAccount(t)
	intentId := rendezvousKey.PublicKey().ToBase58()
	resp, err := p.submitIntent(t, intentId, metadata, actions)
	return submitIntentCallMetadata{
		intentId:      intentId,
		rendezvousKey: rendezvousKey,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) upgradeOneTxnToPermanentPrivacy(t *testing.T, checkStatus bool) submitIntentCallMetadata {
	require.NotEmpty(t, p.txnsToUpgrade)

	txnToUpgrade := p.txnsToUpgrade[0]

	if checkStatus {
		getPrivacyUpgradeStatusResp, err := p.getPrivacyUpgradeStatus(t, txnToUpgrade.intentId, txnToUpgrade.actionId)
		require.NoError(t, err)
		require.Equal(t, transactionpb.GetPrivacyUpgradeStatusResponse_OK, getPrivacyUpgradeStatusResp.Result)
		require.Equal(t, transactionpb.GetPrivacyUpgradeStatusResponse_READY_FOR_UPGRADE, getPrivacyUpgradeStatusResp.Status)
	}

	actions := []*transactionpb.Action{
		{
			Type: &transactionpb.Action_PermanentPrivacyUpgrade{
				PermanentPrivacyUpgrade: &transactionpb.PermanentPrivacyUpgradeAction{
					ActionId: txnToUpgrade.actionId,
				},
			},
		},
	}

	metadata := &transactionpb.Metadata{
		Type: &transactionpb.Metadata_UpgradePrivacy{
			UpgradePrivacy: &transactionpb.UpgradePrivacyMetadata{},
		},
	}

	resp, err := p.submitIntent(t, txnToUpgrade.intentId, metadata, actions)
	if !isSubmitIntentError(resp, err) {
		p.txnsToUpgrade = p.txnsToUpgrade[1:]
	}
	return submitIntentCallMetadata{
		intentId:      txnToUpgrade.intentId,
		protoMetadata: metadata,
		protoActions:  actions,
		resp:          resp,
		err:           err,
	}
}

func (p *phoneTestEnv) getPrivacyUpgradeStatus(t *testing.T, intentId string, actionId uint32) (*transactionpb.GetPrivacyUpgradeStatusResponse, error) {
	intentIdBytes, err := base58.Decode(intentId)
	require.NoError(t, err)

	return p.client.GetPrivacyUpgradeStatus(p.ctx, &transactionpb.GetPrivacyUpgradeStatusRequest{
		IntentId: &commonpb.IntentId{
			Value: intentIdBytes,
		},
		ActionId: actionId,
	})
}

func (p *phoneTestEnv) getPrioritizedIntentsForPrivacyUpgrade(t *testing.T) (*transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse, error) {
	req := &transactionpb.GetPrioritizedIntentsForPrivacyUpgradeRequest{
		Owner: p.parentAccount.ToProto(),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, p.conf.simulateInvalidGetPrioritizedIntentsForPrivacyUpgradeSignature)

	resp, err := p.client.GetPrioritizedIntentsForPrivacyUpgrade(p.ctx, req)
	if err != nil {
		return resp, err
	}

	if resp.Result == transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND {
		assert.Empty(t, resp.Items)
		return resp, err
	}

	require.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)
	assert.NotEmpty(t, resp.Items)
	return resp, err
}

func (p *phoneTestEnv) submitIntent(t *testing.T, intentId string, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (*transactionpb.SubmitIntentResponse, error) {
	submitActionsOwner := p.parentAccount.ToProto()
	var isPrivateTransferIntent, isUpgradeIntent bool
	switch typed := metadata.Type.(type) {
	case *transactionpb.Metadata_OpenAccounts:
		if p.conf.simulateOpeningTooManyAccounts {
			actions = append(
				actions,
				proto.Clone(actions[len(actions)-2]).(*transactionpb.Action),
				proto.Clone(actions[len(actions)-1]).(*transactionpb.Action),
			)
		} else if p.conf.simulateOpeningTooFewAccounts {
			actions = actions[:len(actions)-2]
		}
	case *transactionpb.Metadata_SendPrivatePayment:
		isPrivateTransferIntent = true

		if p.conf.simulateSendingTooLittle {
			typed.SendPrivatePayment.ExchangeData.Quarks += 1
		} else if p.conf.simulateSendingTooMuch {
			typed.SendPrivatePayment.ExchangeData.Quarks -= 1
		}

		if p.conf.simulateInvalidExchangeRate {
			typed.SendPrivatePayment.ExchangeData.ExchangeRate *= 2
			typed.SendPrivatePayment.ExchangeData.NativeAmount *= 2
		} else if p.conf.simulateInvalidNativeAmount {
			typed.SendPrivatePayment.ExchangeData.NativeAmount *= 2
		}

		if p.conf.simulateFundingTempAccountTooMuch {
			actions = append([]*transactionpb.Action{
				{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
						Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}},
			}, actions...)
		}

		if p.conf.simulateNotSendingToDestination {
			typed.SendPrivatePayment.Destination = testutil.NewRandomAccount(t).ToProto()
		}

		if p.conf.simulateFlippingWithdrawFlag {
			typed.SendPrivatePayment.IsWithdrawal = !typed.SendPrivatePayment.IsWithdrawal
		}

		if p.conf.simulateFlippingRemoteSendFlag {
			typed.SendPrivatePayment.IsRemoteSend = !typed.SendPrivatePayment.IsRemoteSend
		}

		if p.conf.simulateFlippingTipFlag {
			typed.SendPrivatePayment.IsTip = true
			typed.SendPrivatePayment.TippedUser = &transactionpb.TippedUser{
				Platform: transactionpb.TippedUser_TWITTER,
				Username: "twitteraccount",
			}
		}

		if p.conf.simulateNotClosingTempAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.ShouldClose = false
				}
			}
		}

		if p.conf.simulateSendPrivatePaymentFromPreviousTempOutgoingAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Destination = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex-1).ToProto()
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex-1).ToProto()
					typed.NoPrivacyWithdraw.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex-1).ToProto()
				}
			}
		}

		if p.conf.simulateSendPrivatePaymentFromTempIncomingAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Destination = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
					typed.NoPrivacyWithdraw.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
				}
			}
		}

		if p.conf.simulateSendPrivatePaymentFromBucketAccount {
			orignalActions := actions
			actions = nil
			for _, action := range orignalActions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Destination = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.Authority = p.getAuthority(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
					typed.NoPrivacyWithdraw.Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
				}
			}

			for _, action := range orignalActions {
				switch action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyExchange:
				default:
					actions = append(actions, action)
				}
			}
		}

		if p.conf.simulateSendPrivatePaymentWithMoreTempOutgoingOutboundTransfers {
			actions = append([]*transactionpb.Action{
				{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
						Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}},
				{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   p.currentDerivedAccounts[commonpb.AccountType_TEMPORARY_OUTGOING].ToProto(),
						Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}},
			}, actions...)
		}

		if p.conf.simulateSendPrivatePaymentWithoutClosingTempIncomingAccount {
			for i, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_NoPrivacyWithdraw:
					actions[i] = &transactionpb.Action{
						Type: &transactionpb.Action_TemporaryPrivacyTransfer{
							TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
								Authority:   typed.NoPrivacyWithdraw.Authority,
								Source:      typed.NoPrivacyWithdraw.Source,
								Destination: typed.NoPrivacyWithdraw.Destination,
								Amount:      typed.NoPrivacyWithdraw.Amount,
							},
						},
					}
				}
			}
		}

		if p.conf.simulateUsingNewTempAccount {
			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex+1).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}})
		}

		if p.conf.simulateClosingWrongAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_NoPrivacyWithdraw:
					*action = transactionpb.Action{
						Type: &transactionpb.Action_TemporaryPrivacyTransfer{
							TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
								Authority:   typed.NoPrivacyWithdraw.Authority,
								Source:      typed.NoPrivacyWithdraw.Source,
								Destination: typed.NoPrivacyWithdraw.Destination,
								Amount:      typed.NoPrivacyWithdraw.Amount,
							},
						},
					}
				}
			}

			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_CloseEmptyAccount{
				CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
					AccountType: commonpb.AccountType_BUCKET_1_000_000_KIN,
					Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_000_000_KIN].ToProto(),
					Token:       p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_000_000_KIN, 0).ToProto(),
				},
			}})
		}

		if p.conf.simulateNotOpeningGiftCardAccount {
			orignalActions := actions
			actions = nil
			for _, action := range orignalActions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						actions = append(actions, action)
					}
				default:
					actions = append(actions, action)
				}
			}
		}

		if p.conf.simulateOpeningWrongGiftCardAccount {
			newAuthority := testutil.NewRandomAccount(t)
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.OpenAccount.Owner = newAuthority.ToProto()
						typed.OpenAccount.Authority = newAuthority.ToProto()
						typed.OpenAccount.Token = getTimelockVault(t, newAuthority).ToProto()
					}
				case *transactionpb.Action_CloseDormantAccount:
					if typed.CloseDormantAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.CloseDormantAccount.Authority = newAuthority.ToProto()
						typed.CloseDormantAccount.Token = getTimelockVault(t, newAuthority).ToProto()
					}
				}
			}
			p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{value: newAuthority})
		}

		if p.conf.simulateOpeningGiftCardWithWrongOwner {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.OpenAccount.Owner = p.parentAccount.ToProto()
					}
				}
			}
		}

		if p.conf.simulateOpeningGiftCardWithUser12Words {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.OpenAccount.Owner = p.parentAccount.ToProto()
						typed.OpenAccount.Authority = p.parentAccount.ToProto()
						typed.OpenAccount.Token = p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto()
					}
				case *transactionpb.Action_CloseDormantAccount:
					typed.CloseDormantAccount.Authority = p.parentAccount.ToProto()
					typed.CloseDormantAccount.Token = p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto()
				}
			}
		}

		if p.conf.simulateOpeningGiftCardAsWrongAccountType {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.OpenAccount.Owner = p.parentAccount.ToProto()
						typed.OpenAccount.AccountType = commonpb.AccountType_PRIMARY
					}
				}
			}
		}

		if p.conf.simulateClosingGiftCardAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						actions = append(actions, &transactionpb.Action{
							Type: &transactionpb.Action_CloseEmptyAccount{
								CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
									AccountType: typed.OpenAccount.AccountType,
									Authority:   typed.OpenAccount.Authority,
									Token:       typed.OpenAccount.Token,
								},
							},
						})
					}
				}
			}
		}

		if p.conf.simulateNotClosingDormantGiftCardAccount {
			orignalActions := actions
			actions = nil
			for _, action := range orignalActions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_CloseDormantAccount:
					if typed.CloseDormantAccount.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						actions = append(actions, action)
					}
				default:
					actions = append(actions, action)
				}
			}
		}

		if p.conf.simulateUsingGiftCardAccount {
			if typed.SendPrivatePayment.IsRemoteSend {
				var giftCardAuthority *commonpb.SolanaAccountId
				for _, action := range actions {
					switch typed := action.Type.(type) {
					case *transactionpb.Action_OpenAccount:
						if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
							giftCardAuthority = typed.OpenAccount.Authority
						}
					}
				}

				if giftCardAuthority == nil {
					require.Fail(t, "no gift card account opened")
				}

				actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   giftCardAuthority,
						Source:      typed.SendPrivatePayment.Destination,
						Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}})
			} else {
				giftCardAccount := p.directServerAccess.generateRandomUnclaimedGiftCard(t)
				actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   giftCardAccount.ToProto(),
						Source:      getTimelockVault(t, giftCardAccount).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}})
			}
		}

		if p.conf.simulateInvalidGiftCardIndex {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_OpenAccount:
					if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.OpenAccount.Index = 1
					}
				}
			}
		}

		if p.conf.simulateClosingDormantGiftCardAsWrongAccountType {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_CloseDormantAccount:
					if typed.CloseDormantAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
						typed.CloseDormantAccount.AccountType = commonpb.AccountType_PRIMARY
					}
				}
			}
		}
	case *transactionpb.Metadata_ReceivePaymentsPrivately:
		isPrivateTransferIntent = true

		if p.conf.simulateReceivingTooLittle {
			typed.ReceivePaymentsPrivately.Quarks += 1
		} else if p.conf.simulateReceivingTooMuch {
			typed.ReceivePaymentsPrivately.Quarks -= 1
		}

		if p.conf.simulateFundingTempAccountTooMuch {
			actions = append([]*transactionpb.Action{
				{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
						Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
						Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
						Amount:      kin.ToQuarks(1),
					},
				}},
			}, actions...)
		}

		if p.conf.simulateNotReceivingFromSource {
			typed.ReceivePaymentsPrivately.Source = testutil.NewRandomAccount(t).ToProto()
		}

		if p.conf.simulateFlippingDepositFlag {
			typed.ReceivePaymentsPrivately.IsDeposit = !typed.ReceivePaymentsPrivately.IsDeposit
		}

		if p.conf.simulateNotClosingTempAccount {
			orignalActions := actions
			actions = nil
			for _, action := range orignalActions {
				switch action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
				default:
					actions = append(actions, action)
				}
			}
		}

		if p.conf.simulateReceivePaymentFromTempOutgoingAccount {
			typed.ReceivePaymentsPrivately.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
					typed.TemporaryPrivacyTransfer.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
				case *transactionpb.Action_CloseEmptyAccount:
					typed.CloseEmptyAccount.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
					typed.CloseEmptyAccount.Token = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
				}
			}
		}

		if p.conf.simulateReceivePaymentFromPreviousTempIncomingAccount {
			typed.ReceivePaymentsPrivately.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex-1).ToProto()
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex-1).ToProto()
					typed.TemporaryPrivacyTransfer.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex-1).ToProto()
				case *transactionpb.Action_CloseEmptyAccount:
					typed.CloseEmptyAccount.Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex-1).ToProto()
					typed.CloseEmptyAccount.Token = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex-1).ToProto()
				}
			}
		}

		if p.conf.simulateReceivePaymentFromBucketAccount {
			typed.ReceivePaymentsPrivately.Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()

			orignalActions := actions
			actions = nil
			for _, action := range orignalActions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_TemporaryPrivacyTransfer:
					typed.TemporaryPrivacyTransfer.Authority = p.getAuthority(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
					typed.TemporaryPrivacyTransfer.Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
					typed.TemporaryPrivacyTransfer.Destination = p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto()
				}
			}

			for _, action := range orignalActions {
				switch action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
				case *transactionpb.Action_TemporaryPrivacyExchange:
				default:
					actions = append(actions, action)
				}
			}
		}

		if p.conf.simulateUsingNewTempAccount {
			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex+1).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}})
		}

		if p.conf.simulateUsingGiftCardAccount {
			giftCardAccount := p.directServerAccess.generateRandomUnclaimedGiftCard(t)
			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   giftCardAccount.ToProto(),
					Source:      getTimelockVault(t, giftCardAccount).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}})
		}

		if p.conf.simulateClosingWrongAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
					typed.CloseEmptyAccount.AccountType = commonpb.AccountType_BUCKET_1_000_000_KIN
					typed.CloseEmptyAccount.Authority = p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_000_000_KIN].ToProto()
					typed.CloseEmptyAccount.Token = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_000_000_KIN, 0).ToProto()
				}
			}
		}
	case *transactionpb.Metadata_UpgradePrivacy:
		isUpgradeIntent = true

		if p.conf.simulateDoublePrivacyUpgradeInSameRequest {
			actions = append(
				actions,
				proto.Clone(actions[0]).(*transactionpb.Action),
			)
		}

		if p.conf.simulateInvalidActionDuringPrivacyUpgrade {
			actions = append(
				actions,
				&transactionpb.Action{
					Type: &transactionpb.Action_CloseEmptyAccount{
						CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
							AccountType: commonpb.AccountType_BUCKET_100_KIN,
							Authority:   testutil.NewRandomAccount(t).ToProto(),
							Token:       testutil.NewRandomAccount(t).ToProto(),
						},
					},
				},
			)
		}
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		legacyTimelockAccounts, err := p.parentAccount.GetTimelockAccounts(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
		require.NoError(t, err)

		if p.conf.simulateClosingWrongAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
					typed.CloseEmptyAccount.AccountType = commonpb.AccountType_PRIMARY
					typed.CloseEmptyAccount.Authority = p.parentAccount.ToProto()
					typed.CloseEmptyAccount.Token = p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto()
				}
			}
		}

		if p.conf.simulateMigratingFundsToWrongAccount {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.Destination = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).ToProto()
				}
			}
		}

		if p.conf.simulatingMigratingWithWrongAmountInAction {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_NoPrivacyWithdraw:
					typed.NoPrivacyWithdraw.Amount += 1
				}
			}
		}

		if p.conf.simulateMigratingUsingOppositeAction {
			var newActions []*transactionpb.Action
			for _, action := range actions {
				switch action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
					newActions = append(newActions, &transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
						NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
							Authority:   p.parentAccount.ToProto(),
							Source:      legacyTimelockAccounts.Vault.ToProto(),
							Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
							Amount:      kin.ToQuarks(1),
							ShouldClose: true,
						},
					}})
				case *transactionpb.Action_NoPrivacyWithdraw:
					newActions = append(newActions, &transactionpb.Action{Type: &transactionpb.Action_CloseEmptyAccount{
						CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
							AccountType: commonpb.AccountType_LEGACY_PRIMARY_2022,
							Authority:   p.parentAccount.ToProto(),
							Token:       legacyTimelockAccounts.Vault.ToProto(),
						},
					}})
				}
			}
			actions = newActions
		}

		if p.conf.simulateMigratingUsingWrongAction {
			var newActions []*transactionpb.Action
			for _, action := range actions {
				switch action.Type.(type) {
				case *transactionpb.Action_CloseEmptyAccount:
					newActions = append(newActions, &transactionpb.Action{Type: &transactionpb.Action_CloseDormantAccount{
						CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
							AccountType: commonpb.AccountType_LEGACY_PRIMARY_2022,
							Authority:   p.parentAccount.ToProto(),
							Token:       legacyTimelockAccounts.Vault.ToProto(),
							Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
						},
					}})
				case *transactionpb.Action_NoPrivacyWithdraw:
					newActions = append(newActions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
						TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
							Authority:   p.parentAccount.ToProto(),
							Source:      legacyTimelockAccounts.Vault.ToProto(),
							Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
							Amount:      kin.ToQuarks(1),
						},
					}})
				}
			}
			actions = newActions
		}
	case *transactionpb.Metadata_SendPublicPayment:
		if p.conf.simulateSendingTooLittle {
			typed.SendPublicPayment.ExchangeData.Quarks += 1
		} else if p.conf.simulateSendingTooMuch {
			typed.SendPublicPayment.ExchangeData.Quarks -= 1
		}

		if p.conf.simulateInvalidExchangeRate {
			typed.SendPublicPayment.ExchangeData.ExchangeRate *= 2
			typed.SendPublicPayment.ExchangeData.NativeAmount *= 2
		} else if p.conf.simulateInvalidNativeAmount {
			typed.SendPublicPayment.ExchangeData.NativeAmount *= 2
		}

		if p.conf.simulateNotSendingToDestination {
			typed.SendPublicPayment.Destination = testutil.NewRandomAccount(t).ToProto()
		}

		if p.conf.simulateFlippingWithdrawFlag {
			typed.SendPublicPayment.IsWithdrawal = !typed.SendPublicPayment.IsWithdrawal
		}

		if p.conf.simulateSendPublicPaymentPrivately {
			actions[0] = &transactionpb.Action{
				Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   actions[0].GetNoPrivacyTransfer().Authority,
						Source:      actions[0].GetNoPrivacyTransfer().Source,
						Destination: actions[0].GetNoPrivacyTransfer().Destination,
						Amount:      actions[0].GetNoPrivacyTransfer().Amount,
					},
				},
			}
		}

		if p.conf.simulateSendPublicPaymentWithWithdraw {
			actions[0] = &transactionpb.Action{
				Type: &transactionpb.Action_NoPrivacyWithdraw{
					NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
						Authority:   actions[0].GetNoPrivacyTransfer().Authority,
						Source:      actions[0].GetNoPrivacyTransfer().Source,
						Destination: actions[0].GetNoPrivacyTransfer().Destination,
						Amount:      actions[0].GetNoPrivacyTransfer().Amount,
						ShouldClose: true,
					},
				},
			}
		}

		if p.conf.simulateSendPublicPaymentFromBucketAccount {
			metadata.GetSendPublicPayment().Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
			actions[0].GetNoPrivacyTransfer().Authority = p.getAuthority(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
			actions[0].GetNoPrivacyTransfer().Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
		}

		if p.conf.simulateUsingGiftCardAccount {
			giftCardAccount := p.directServerAccess.generateRandomUnclaimedGiftCard(t)
			metadata.GetSendPublicPayment().Source = getTimelockVault(t, giftCardAccount).ToProto()
			actions[0].GetNoPrivacyTransfer().Authority = giftCardAccount.ToProto()
			actions[0].GetNoPrivacyTransfer().Source = getTimelockVault(t, giftCardAccount).ToProto()
		}

		if p.conf.simulateNotSendingFromSource {
			actions[0].GetNoPrivacyTransfer().Authority = p.getAuthority(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
			actions[0].GetNoPrivacyTransfer().Source = p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto()
		}
	case *transactionpb.Metadata_ReceivePaymentsPublicly:
		if p.conf.simulateReceivingFromDesktop {
			submitActionsOwner = actions[0].GetNoPrivacyWithdraw().Authority
		}

		if p.conf.simulateFlippingRemoteSendFlag {
			typed.ReceivePaymentsPublicly.IsRemoteSend = !typed.ReceivePaymentsPublicly.IsRemoteSend
		}

		if typed.ReceivePaymentsPublicly.IsRemoteSend && p.conf.simulateClaimingTooLittleFromGiftCard {
			typed.ReceivePaymentsPublicly.Quarks += 1
		} else if typed.ReceivePaymentsPublicly.IsRemoteSend && p.conf.simulateClaimingTooMuchFromGiftCard {
			typed.ReceivePaymentsPublicly.Quarks -= 1
		}

		if p.conf.simulateReceivingTooLittle {
			actions[0].GetNoPrivacyWithdraw().Amount -= 1
		} else if p.conf.simulateReceivingTooMuch {
			actions[0].GetNoPrivacyWithdraw().Amount += 1
		}

		if p.conf.simulateNotReceivingFromSource {
			actions[0].GetNoPrivacyWithdraw().Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempIncomingIndex).ToProto()
			actions[0].GetNoPrivacyWithdraw().Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempIncomingIndex).ToProto()
		}

		if p.conf.simulateNotSendingToDestination {
			actions[0].GetNoPrivacyWithdraw().Destination = testutil.NewRandomAccount(t).ToProto()
		}

		if p.conf.simulateUsingCurrentTempIncomingAccountAsSource {
			typed.ReceivePaymentsPublicly.Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
			actions[0].GetNoPrivacyWithdraw().Authority = p.getAuthority(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
			actions[0].GetNoPrivacyWithdraw().Source = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto()
		}

		if p.conf.simulateUsingPrimaryAccountAsSource {
			typed.ReceivePaymentsPublicly.Source = p.parentAccount.ToProto()
			actions[0].GetNoPrivacyWithdraw().Authority = p.parentAccount.ToProto()
			actions[0].GetNoPrivacyWithdraw().Source = p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto()
		}

		if p.conf.simulateUsingPrimaryAccountAsDestination {
			actions[0].GetNoPrivacyWithdraw().Destination = p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto()
		}

		if p.conf.simulateUsingCurrentTempOutgoingAccountAsDestination {
			actions[0].GetNoPrivacyWithdraw().Destination = p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto()
		}

		if p.conf.simulateReceivePaymentsPubliclyWithoutWithdraw {
			actions[0] = &transactionpb.Action{
				Type: &transactionpb.Action_NoPrivacyTransfer{
					NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
						Authority:   actions[0].GetNoPrivacyWithdraw().Authority,
						Source:      actions[0].GetNoPrivacyWithdraw().Source,
						Destination: actions[0].GetNoPrivacyWithdraw().Destination,
						Amount:      actions[0].GetNoPrivacyWithdraw().Amount,
					},
				},
			}
		}

		if p.conf.simulateReceivePaymentsPubliclyPrivately {
			actions[0] = &transactionpb.Action{
				Type: &transactionpb.Action_TemporaryPrivacyTransfer{
					TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
						Authority:   actions[0].GetNoPrivacyWithdraw().Authority,
						Source:      actions[0].GetNoPrivacyWithdraw().Source,
						Destination: actions[0].GetNoPrivacyWithdraw().Destination,
						Amount:      actions[0].GetNoPrivacyWithdraw().Amount,
					},
				},
			}
		}
	case *transactionpb.Metadata_EstablishRelationship:
		if p.conf.simulateDerivingDifferentRelationshipAccount {
			authority := testutil.NewRandomAccount(t)
			actions[0].GetOpenAccount().Authority = authority.ToProto()
			actions[0].GetOpenAccount().Token = getTimelockVault(t, authority).ToProto()
			p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{
				accountType:    commonpb.AccountType_RELATIONSHIP,
				value:          authority,
				relationshipTo: &typed.EstablishRelationship.Relationship.GetDomain().Value,
			})
		}

		if p.conf.simulateOpeningTooManyAccounts {
			actions = append(
				actions,
				proto.Clone(actions[0]).(*transactionpb.Action),
			)
			authority := testutil.NewRandomAccount(t)
			actions[1].GetOpenAccount().Authority = authority.ToProto()
			actions[1].GetOpenAccount().Token = getTimelockVault(t, authority).ToProto()
			p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{
				accountType:    commonpb.AccountType_RELATIONSHIP,
				relationshipTo: &typed.EstablishRelationship.Relationship.GetDomain().Value,
				value:          authority,
			})
		} else if p.conf.simulateOpeningTooFewAccounts {
			actions = nil
		}
	}

	for _, action := range actions {
		switch typed := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			if p.conf.simulateOpenPrimaryAccountActionReplaced && typed.OpenAccount.AccountType == commonpb.AccountType_PRIMARY {
				*action = transactionpb.Action{
					Type: &transactionpb.Action_PermanentPrivacyUpgrade{},
				}
				continue
			}

			if p.conf.simulateOpenNonPrimaryAccountActionReplaced && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				*action = transactionpb.Action{
					Type: &transactionpb.Action_PermanentPrivacyUpgrade{},
				}
				continue
			}

			if p.conf.simulateInvalidAccountTypeForOpenPrimaryAccountAction && typed.OpenAccount.AccountType == commonpb.AccountType_PRIMARY {
				typed.OpenAccount.AccountType = commonpb.AccountType_BUCKET_1_KIN
			}

			if p.conf.simulateInvalidAuthorityForOpenPrimaryAccountAction && typed.OpenAccount.AccountType == commonpb.AccountType_PRIMARY {
				newAuthority := testutil.NewRandomAccount(t)
				typed.OpenAccount.Authority = newAuthority.ToProto()
				typed.OpenAccount.Token = getTimelockVault(t, newAuthority).ToProto()
				p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{value: newAuthority})
			}

			if p.conf.simulateInvalidTokenAccountForOpenPrimaryAccountAction && typed.OpenAccount.AccountType == commonpb.AccountType_PRIMARY {
				typed.OpenAccount.Token = testutil.NewRandomAccount(t).ToProto()
			}

			if p.conf.simulateInvalidIndexForOpenPrimaryAccountAction && typed.OpenAccount.AccountType == commonpb.AccountType_PRIMARY {
				typed.OpenAccount.Index += 1
			}

			if p.conf.simulateInvalidAccountTypeForOpenNonPrimaryAccountAction && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				typed.OpenAccount.AccountType = commonpb.AccountType_PRIMARY
			}

			if p.conf.simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				typed.OpenAccount.Authority = p.parentAccount.ToProto()
				typed.OpenAccount.Token = getTimelockVault(t, p.parentAccount).ToProto()
			}

			if p.conf.simulateInvalidAuthorityForOpenNonPrimaryAccountAction && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				newAuthority := testutil.NewRandomAccount(t)
				typed.OpenAccount.Authority = newAuthority.ToProto()
				p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{value: newAuthority})
			}

			if p.conf.simulateInvalidTokenAccountForOpenNonPrimaryAccountAction && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				typed.OpenAccount.Token = testutil.NewRandomAccount(t).ToProto()
			}

			if p.conf.simulateInvalidIndexForOpenNonPrimaryAccountAction && typed.OpenAccount.AccountType != commonpb.AccountType_PRIMARY {
				typed.OpenAccount.Index += 1
			}

			if p.conf.simulateOpeningWrongTempAccount && typed.OpenAccount.AccountType == commonpb.AccountType_TEMPORARY_INCOMING {
				typed.OpenAccount.AccountType = commonpb.AccountType_TEMPORARY_OUTGOING
			} else if p.conf.simulateOpeningWrongTempAccount && typed.OpenAccount.AccountType == commonpb.AccountType_TEMPORARY_OUTGOING {
				typed.OpenAccount.AccountType = commonpb.AccountType_TEMPORARY_INCOMING
			}
		case *transactionpb.Action_CloseEmptyAccount:
			if p.conf.simulateInvalidTokenAccountForCloseEmptyAccountAction {
				typed.CloseEmptyAccount.Token = testutil.NewRandomAccount(t).ToProto()
			}
			if p.conf.simulateInvalidAuthorityForCloseEmptyAccountAction {
				typed.CloseEmptyAccount.Authority = testutil.NewRandomAccount(t).ToProto()
			}
		case *transactionpb.Action_CloseDormantAccount:
			if p.conf.simulateCloseDormantAccountActionReplaced {
				*action = transactionpb.Action{
					Type: &transactionpb.Action_PermanentPrivacyUpgrade{},
				}
				continue
			}

			if p.conf.simulateInvalidAccountTypeForCloseDormantAccountAction {
				typed.CloseDormantAccount.AccountType = commonpb.AccountType_PRIMARY
			}

			if p.conf.simulateInvalidAuthorityForCloseDormantAccountAction {
				newAuthority := testutil.NewRandomAccount(t)
				typed.CloseDormantAccount.Authority = newAuthority.ToProto()
				typed.CloseDormantAccount.Token = getTimelockVault(t, newAuthority).ToProto()
				p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{value: newAuthority})
			}

			if p.conf.simulateInvalidTokenAccountForCloseDormantAccountAction {
				typed.CloseDormantAccount.Token = testutil.NewRandomAccount(t).ToProto()
			}

			if p.conf.simulateInvalidDestitinationForCloseDormantAccountAction {
				typed.CloseDormantAccount.Destination = testutil.NewRandomAccount(t).ToProto()
			}

			if p.conf.simulateOpeningWrongTempAccount && typed.CloseDormantAccount.AccountType == commonpb.AccountType_TEMPORARY_INCOMING {
				typed.CloseDormantAccount.AccountType = commonpb.AccountType_TEMPORARY_OUTGOING
			} else if p.conf.simulateOpeningWrongTempAccount && typed.CloseDormantAccount.AccountType == commonpb.AccountType_TEMPORARY_OUTGOING {
				typed.CloseDormantAccount.AccountType = commonpb.AccountType_TEMPORARY_INCOMING
			}
		case *transactionpb.Action_NoPrivacyTransfer:
			if p.conf.simulateInvalidTokenAccountForNoPrivacyTransferAction {
				typed.NoPrivacyTransfer.Authority = testutil.NewRandomAccount(t).ToProto()
			}
		case *transactionpb.Action_NoPrivacyWithdraw:
			if p.conf.simulateInvalidTokenAccountForNoPrivacyWithdrawAction {
				typed.NoPrivacyWithdraw.Authority = testutil.NewRandomAccount(t).ToProto()
			}
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			if p.conf.simulateInvalidTokenAccountForTemporaryPrivacyTransferAction {
				typed.TemporaryPrivacyTransfer.Authority = testutil.NewRandomAccount(t).ToProto()
			}
		case *transactionpb.Action_TemporaryPrivacyExchange:
			if p.conf.simulateInvalidTokenAccountForTemporaryPrivacyExchangeAction {
				typed.TemporaryPrivacyExchange.Authority = testutil.NewRandomAccount(t).ToProto()
			}
		case *transactionpb.Action_PermanentPrivacyUpgrade:
		}
	}

	if p.conf.simulateUsingPrimaryAccountAsSource {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.parentAccount.ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(1),
			},
		}})
	}

	if p.conf.simulateUsingPrimaryAccountAsDestination && isPrivateTransferIntent {
		if metadata.GetReceivePaymentsPrivately() != nil && metadata.GetReceivePaymentsPrivately().IsDeposit {
			metadata.GetReceivePaymentsPrivately().Quarks -= kin.ToQuarks(1)
		}

		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.getDerivedAccount(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
				Amount:      kin.ToQuarks(1),
			},
		}})
	}

	if p.conf.simulateUsingCurrentTempIncomingAccountAsSource && isPrivateTransferIntent {
		actions = append([]*transactionpb.Action{{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
				Amount:      kin.ToQuarks(1),
			},
		}}}, actions...)
	}

	if p.conf.simulateUsingCurrentTempIncomingAccountAsDestination && isPrivateTransferIntent {
		actions = append([]*transactionpb.Action{
			{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   p.getDerivedAccount(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}},
			{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   p.getDerivedAccount(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, p.currentTempIncomingIndex).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}},
		}, actions...)
	}

	if p.conf.simulateUsingCurrentTempOutgoingAccountAsDestination && isPrivateTransferIntent {
		actions = append([]*transactionpb.Action{{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, p.currentTempOutgoingIndex).ToProto(),
				Amount:      kin.ToQuarks(1),
			},
		}}}, actions...)
	}

	if p.conf.simulateRandomOpenAccountActionInjected {
		newAuthority := testutil.NewRandomAccount(t)
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_OpenAccount{
				OpenAccount: &transactionpb.OpenAccountAction{
					AccountType: commonpb.AccountType_PRIMARY,
					Owner:       p.parentAccount.ToProto(),
					Authority:   newAuthority.ToProto(),
					Token:       getTimelockVault(t, newAuthority).ToProto(),
				},
			},
		})
		p.allDerivedAccounts = append(p.allDerivedAccounts, derivedAccount{value: newAuthority})
	}

	if p.conf.simulateRandomCloseEmptyAccountActionInjected {
		authority, _ := p.getAuthorityForLatestAccount(t, commonpb.AccountType_BUCKET_1_000_000_KIN)
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_CloseEmptyAccount{
				CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
					AccountType: commonpb.AccountType_BUCKET_1_000_000_KIN,
					Authority:   authority.ToProto(),
					Token:       getTimelockVault(t, authority).ToProto(),
				},
			},
		})
	}

	if p.conf.simulateRandomCloseDormantAccountActionInjected {
		newAuthority := testutil.NewRandomAccount(t)
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_CloseDormantAccount{
				CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
					AccountType: commonpb.AccountType_PRIMARY,
					Authority:   newAuthority.ToProto(),
					Token:       getTimelockVault(t, newAuthority).ToProto(),
					Destination: testutil.NewRandomAccount(t).ToProto(),
				},
			},
		})
	}

	if p.conf.simulatePrivacyUpgradeActionInjected {
		actions = append(actions, &transactionpb.Action{
			Type: &transactionpb.Action_PermanentPrivacyUpgrade{},
		})
	}

	if p.conf.simulateInvalidBucketExchangeMultiple {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_100_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(99),
			},
		}})
	}

	if p.conf.simulateInvalidAnyonymizedBucketAmount {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_000_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_000_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_000_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(10_000_000),
			},
		}})
	}

	if p.conf.simulateBucketExchangeInATransfer {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(100),
			},
		}})
	}

	if p.conf.simulateBucketExchangeInAPublicTransfer {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(100),
			},
		}})
	}

	if p.conf.simulateBucketExchangeInAPublicWithdraw {
		actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_10_KIN].ToProto(),
				Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
				Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0).ToProto(),
				Amount:      kin.ToQuarks(100),
				ShouldClose: true,
			},
		}})
	}

	if p.conf.simulateSwappingTransfersForExchanges {
		for _, action := range actions {
			switch typed := action.Type.(type) {
			case *transactionpb.Action_TemporaryPrivacyTransfer:
				*action = transactionpb.Action{
					Type: &transactionpb.Action_TemporaryPrivacyExchange{
						TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
							Authority:   typed.TemporaryPrivacyTransfer.Authority,
							Source:      typed.TemporaryPrivacyTransfer.Source,
							Destination: typed.TemporaryPrivacyTransfer.Destination,
							Amount:      typed.TemporaryPrivacyTransfer.Amount,
						},
					},
				}
			}
		}
	}

	if p.conf.simulateSwappingExchangesForTransfers {
		for _, action := range actions {
			switch typed := action.Type.(type) {
			case *transactionpb.Action_TemporaryPrivacyExchange:
				*action = transactionpb.Action{
					Type: &transactionpb.Action_TemporaryPrivacyTransfer{
						TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
							Authority:   typed.TemporaryPrivacyExchange.Authority,
							Source:      typed.TemporaryPrivacyExchange.Source,
							Destination: typed.TemporaryPrivacyExchange.Destination,
							Amount:      typed.TemporaryPrivacyExchange.Amount,
						},
					},
				}
			}
		}
	}

	if p.conf.simulateTooManyTemporaryPrivacyTransfers {
		for i := 0; i < 50; i++ {
			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyTransfer{
				TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
					Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).ToProto(),
					Amount:      kin.ToQuarks(1),
				},
			}})
		}
	}

	if p.conf.simulateTooManyTemporaryPrivacyExchanges {
		for i := 0; i < 50; i++ {
			actions = append(actions, &transactionpb.Action{Type: &transactionpb.Action_TemporaryPrivacyExchange{
				TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
					Authority:   p.currentDerivedAccounts[commonpb.AccountType_BUCKET_1_KIN].ToProto(),
					Source:      p.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0).ToProto(),
					Destination: p.getTimelockVault(t, commonpb.AccountType_BUCKET_10_KIN, 0).ToProto(),
					Amount:      kin.ToQuarks(10),
				},
			}})
		}
	}

	for id, action := range actions {
		action.Id = uint32(id)

		switch typed := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			if p.conf.simulateInvalidOpenAccountOwner {
				typed.OpenAccount.Owner = testutil.NewRandomAccount(t).ToProto()
			}

			typed.OpenAccount.AuthoritySignature = p.signProtoMessage(t, typed.OpenAccount, typed.OpenAccount.Authority, p.conf.simulateInvalidOpenAccountSignature)
		}
	}

	if p.conf.simulateInvalidActionId {
		require.True(t, len(actions) >= 2)

		actions[0].Id += 1
		actions[1].Id -= 1
	}

	if p.conf.simulateReusingIntentId {
		intentId = p.lastIntentSubmitted
	}

	if isUpgradeIntent && p.conf.simulateUpgradeToNonExistantIntentId {
		intentId = testutil.NewRandomAccount(t).PublicKey().ToBase58()
	}

	if p.conf.simulatePaymentRequest {
		var exchangeData *transactionpb.ExchangeData
		var destinationTokenAccount *commonpb.SolanaAccountId
		switch typed := metadata.Type.(type) {
		case *transactionpb.Metadata_SendPrivatePayment:
			exchangeData = typed.SendPrivatePayment.ExchangeData
			destinationTokenAccount = typed.SendPrivatePayment.Destination
		default:
			// For all other intents, just populate anything
			exchangeData = &transactionpb.ExchangeData{
				Currency:     "usd",
				NativeAmount: 88.8,
			}
			destinationTokenAccount = testutil.NewRandomAccount(t).ToProto()
		}

		// Simulate a payment request flow by saving the equivalent record that
		// would be created during the rendezvous process
		paymentRequestRecord := &paymentrequest.Record{
			Intent: intentId,

			DestinationTokenAccount: pointer.String(base58.Encode(destinationTokenAccount.Value)),
			ExchangeCurrency:        &exchangeData.Currency,
			NativeAmount:            &exchangeData.NativeAmount,

			Domain:     pointer.String("example.com"),
			IsVerified: true,

			CreatedAt: time.Now(),
		}
		for _, action := range actions {
			switch typed := action.Type.(type) {
			case *transactionpb.Action_FeePayment:
				if typed.FeePayment.Type == transactionpb.FeePaymentAction_THIRD_PARTY {
					destination := base58.Encode(typed.FeePayment.Destination.Value)
					if p.conf.simulateInvalidThirdPartyFeeDestination {
						destination = testutil.NewRandomAccount(t).PublicKey().ToBase58()
					}

					bps := defaultTestThirdPartyFeeBps
					if p.conf.simulateInvalidThirdPartyFeeAmount {
						bps += 1
					}

					paymentRequestRecord.Fees = append(paymentRequestRecord.Fees, &paymentrequest.Fee{
						DestinationTokenAccount: destination,
						BasisPoints:             uint16(bps),
					})
				}
			}
		}
		if p.conf.simulateTooManyThirdPartyFees {
			paymentRequestRecord.Fees = paymentRequestRecord.Fees[:len(paymentRequestRecord.Fees)-1]
		} else if p.conf.simulateThirdPartyFeeMissing {
			paymentRequestRecord.Fees = append(paymentRequestRecord.Fees, &paymentrequest.Fee{
				DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
				BasisPoints:             defaultTestThirdPartyFeeBps,
			})
		}
		if p.conf.simulateLoginRequest {
			// Simulate a login request by downgrading the payment request to having no payment
			paymentRequestRecord.DestinationTokenAccount = nil
			paymentRequestRecord.ExchangeCurrency = nil
			paymentRequestRecord.NativeAmount = nil
			paymentRequestRecord.Fees = nil
		}
		if p.conf.simulateUnverifiedPaymentRequest {
			paymentRequestRecord.IsVerified = false
		}

		// Update the payment request to something else if we need to test the
		// client submitting something different than the expectation
		if p.conf.simulateInvalidPaymentRequestDestination {
			paymentRequestRecord.DestinationTokenAccount = pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58())
		}
		if p.conf.simulateInvalidPaymentRequestExchangeCurrency {
			paymentRequestRecord.ExchangeCurrency = pointer.String(string(currency_lib.AED))
		}
		if p.conf.simulateInvalidPaymentRequestNativeAmount {
			paymentRequestRecord.NativeAmount = pointer.Float64(*paymentRequestRecord.NativeAmount + 0.01)
		}

		require.NoError(t, p.directServerAccess.data.CreateRequest(p.directServerAccess.ctx, paymentRequestRecord))

		if p.conf.simulateCodeFeeNotPaid {
			for i, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_FeePayment:
					actions[i+1].GetNoPrivacyWithdraw().Amount += typed.FeePayment.Amount
					actions = append(actions[:i], actions[i+1:]...)
				}
			}
			for i, action := range actions {
				action.Id = uint32(i)
			}
		}

		if p.conf.simulateCodeFeeAsThirdParyFee {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_FeePayment:
					if typed.FeePayment.Type == transactionpb.FeePaymentAction_CODE {
						typed.FeePayment.Type = transactionpb.FeePaymentAction_THIRD_PARTY
						typed.FeePayment.Destination = testutil.NewRandomAccount(t).ToProto()
					}
				}
			}
		}
		if p.conf.simulateThirdParyFeeAsCodeFee {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_FeePayment:
					if typed.FeePayment.Type == transactionpb.FeePaymentAction_THIRD_PARTY {
						typed.FeePayment.Type = transactionpb.FeePaymentAction_CODE
						typed.FeePayment.Destination = nil
					}
				}
			}
		}

		if p.conf.simulateMultipleCodeFeePayments {
			actions = append(actions, &transactionpb.Action{
				Id: uint32(len(actions)),
				Type: &transactionpb.Action_FeePayment{
					FeePayment: &transactionpb.FeePaymentAction{
						Type:      transactionpb.FeePaymentAction_CODE,
						Authority: p.parentAccount.ToProto(),
						Source:    getTimelockVault(t, p.parentAccount).ToProto(),
						Amount:    1,
					},
				},
			})
		}

		if p.conf.simulateThirdPartyFeeDestinationMissing {
			for _, action := range actions {
				switch typed := action.Type.(type) {
				case *transactionpb.Action_FeePayment:
					if typed.FeePayment.Type == transactionpb.FeePaymentAction_THIRD_PARTY {
						typed.FeePayment.Destination = nil
					}
				}
			}
		}

		for i, action := range actions {
			deltaFee := kin.ToQuarks(1) / 100 // 0.01 Kin
			switch typed := action.Type.(type) {
			case *transactionpb.Action_FeePayment:
				if p.conf.simulateSmallCodeFee {
					typed.FeePayment.Amount -= deltaFee
					actions[i+1].GetNoPrivacyWithdraw().Amount += deltaFee
				} else if p.conf.simulateLargeCodeFee {
					typed.FeePayment.Amount += deltaFee
					actions[i+1].GetNoPrivacyWithdraw().Amount -= deltaFee
				}
			}
		}
	}
	if p.conf.simulateCodeFeePaid {
		actions = append(actions, &transactionpb.Action{
			Id: uint32(len(actions)),
			Type: &transactionpb.Action_FeePayment{
				FeePayment: &transactionpb.FeePaymentAction{
					Type:      transactionpb.FeePaymentAction_CODE,
					Authority: p.parentAccount.ToProto(),
					Source:    getTimelockVault(t, p.parentAccount).ToProto(),
					Amount:    1,
				},
			},
		})
	}

	intentIdBytes, err := base58.Decode(intentId)
	require.NoError(t, err)

	submitActionsReq := &transactionpb.SubmitIntentRequest_SubmitActions{
		Id: &commonpb.IntentId{
			Value: intentIdBytes,
		},
		Owner:    submitActionsOwner,
		Metadata: metadata,
		Actions:  actions,
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	}
	if p.conf.simulateNoDeviceTokenProvided {
		submitActionsReq.DeviceToken = nil
	} else if p.conf.simualteInvalidDeviceTokenProvided {
		submitActionsReq.DeviceToken.Value = memory_device_verifier.InvalidDeviceToken
	}
	submitActionsReq.Signature = p.signProtoMessage(t, submitActionsReq, submitActionsReq.Owner, p.conf.simulateInvalidSubmitIntentRequestSignature)

	// Start the RPC
	streamer, err := p.client.SubmitIntent(p.ctx)
	require.NoError(t, err)

	if p.conf.simulateDelayForSubmittingActions {
		time.Sleep(250 * time.Millisecond)
	}

	// Send SubmitActions
	err = streamer.Send(&transactionpb.SubmitIntentRequest{
		Request: &transactionpb.SubmitIntentRequest_SubmitActions_{
			SubmitActions: submitActionsReq,
		},
	})
	if err != nil {
		return nil, err
	}

	// Receive ServerParameters or success no-op
	resp, err := streamer.Recv()
	if err != nil || resp.GetError() != nil {
		return resp, err
	}

	// Is it a success with no signatures required, or a no-op?
	successResp := resp.GetSuccess()
	if successResp != nil {
		assert.Equal(t, transactionpb.SubmitIntentResponse_Success_OK, successResp.Code)

		// Close the RPC on the client side
		require.NoError(t, streamer.CloseSend())

		return resp, nil
	}

	// If not, we must receive server parameters
	serverParametersResp := resp.GetServerParameters()
	require.NotNil(t, serverParametersResp)
	assert.Len(t, serverParametersResp.ServerParameters, len(submitActionsReq.Actions))

	// Iterate through all the actions and identify those that require at least one
	// client signature. Clients use a well-known construction heuristic using client
	// and server parameters to make the transaction for signing.
	var protoSignatures []*commonpb.Signature
	for i, action := range actions {
		// Note: Currently at most one transaction to sign per action
		var txnToSign *txnToSign

		// Implementations that construct and sign the transactions have duplicated
		// code from server and are a bit ugly. However, it's representative of the
		// exact logic that will be executed on the client.
		//
		// todo: Fix duplication between private transfer and exchanges
		switch typed := action.Type.(type) {
		case *transactionpb.Action_CloseEmptyAccount:
			txn := p.getCloseEmptyAccountTransactionToSign(
				t,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.CloseEmptyAccount,
				metadata,
			)
			txnToSign = &txn
		case *transactionpb.Action_CloseDormantAccount:
			txn := p.getCloseDormantAccountTransactionToSign(
				t,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.CloseDormantAccount,
			)
			txnToSign = &txn
		case *transactionpb.Action_NoPrivacyTransfer:
			txn := p.getNoPrivacyTransferTransactionToSign(
				t,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.NoPrivacyTransfer,
				metadata,
			)
			txnToSign = &txn
		case *transactionpb.Action_NoPrivacyWithdraw:
			txn := p.getNoPrivacyWithdrawTransactionToSign(
				t,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.NoPrivacyWithdraw,
				metadata,
			)
			txnToSign = &txn
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			txn, upgradeMetadata := p.getTemporaryPrivacyTransferTransactionToSign(
				t,
				intentId,
				action.Id,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.TemporaryPrivacyTransfer,
				serverParametersResp.ServerParameters[i].GetTemporaryPrivacyTransfer(),
			)

			txnToSign = &txn
			p.txnsToUpgrade = append(p.txnsToUpgrade, upgradeMetadata)
		case *transactionpb.Action_TemporaryPrivacyExchange:
			txn, upgradeMetadata := p.getTemporaryPrivacyExchangeTransactionToSign(
				t,
				intentId,
				action.Id,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.TemporaryPrivacyExchange,
				serverParametersResp.ServerParameters[i].GetTemporaryPrivacyExchange(),
			)

			txnToSign = &txn
			p.txnsToUpgrade = append(p.txnsToUpgrade, upgradeMetadata)
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			txn := p.getPermanentPrivacyTransactionToSign(
				t,
				intentId,
				typed.PermanentPrivacyUpgrade,
				serverParametersResp.ServerParameters[i].GetPermanentPrivacyUpgrade(),
			)
			txnToSign = &txn
		case *transactionpb.Action_FeePayment:
			txn := p.getFeePaymentTransactionToSign(
				t,
				serverParametersResp.ServerParameters[i].Nonces[0],
				typed.FeePayment,
				serverParametersResp.ServerParameters[i].GetFeePayment(),
			)
			txnToSign = &txn
		default:
			// Client doesn't care. There are no transactions for this action
			// to sign.
		}

		if txnToSign != nil {
			signer := p.getSigner(t, txnToSign.authority.PublicKey().ToBytes())
			signature := ed25519.Sign(signer.PrivateKey().ToBytes(), txnToSign.txn.Message.Marshal())
			protoSignatures = append(protoSignatures, &commonpb.Signature{
				Value: signature,
			})
		}
	}

	if p.conf.simulateTooManySubmittedSignatures {
		protoSignatures = append(protoSignatures, protoSignatures[0])
	} else if p.conf.simulateTooFewSubmittedSignatures {
		protoSignatures = protoSignatures[:len(protoSignatures)-1]
	}

	if p.conf.simulateInvalidSignatureValueSubmitted {
		protoSignatures[0].Value[0] += 1
	}

	if p.conf.simulateDelayForSubmittingSignatures {
		time.Sleep(250 * time.Millisecond)
	}

	// Send SubmitSignatures
	submitSignaturesReq := &transactionpb.SubmitIntentRequest{
		Request: &transactionpb.SubmitIntentRequest_SubmitSignatures_{
			SubmitSignatures: &transactionpb.SubmitIntentRequest_SubmitSignatures{
				Signatures: protoSignatures,
			},
		},
	}
	err = streamer.Send(submitSignaturesReq)
	if err != nil {
		return nil, err
	}

	// Receive final result
	resp, err = streamer.Recv()

	require.NoError(t, err)
	if resp.GetError() != nil {
		return resp, nil
	}

	successResp = resp.GetSuccess()
	require.NotNil(t, successResp)
	assert.Equal(t, transactionpb.SubmitIntentResponse_Success_OK, successResp.Code)

	p.lastIntentSubmitted = intentId

	// Close the RPC on the client side
	require.NoError(t, streamer.CloseSend())

	return resp, nil
}

func (p *phoneTestEnv) getIntentMetadata(t *testing.T, rendezvousKey *common.Account, signAsOwner bool) *transactionpb.GetIntentMetadataResponse {
	req := &transactionpb.GetIntentMetadataRequest{
		IntentId: &commonpb.IntentId{
			Value: rendezvousKey.ToProto().Value,
		},
	}

	if signAsOwner {
		req.Owner = p.parentAccount.ToProto()
		req.Signature = p.signProtoMessage(t, req, req.Owner, false)
	} else {
		req.Signature = p.signProtoMessageWithSigner(t, req, rendezvousKey, false)
	}

	resp, err := p.client.GetIntentMetadata(p.ctx, req)
	require.NoError(t, err)
	return resp
}

func (p *phoneTestEnv) requestAirdrop(t *testing.T, airdropType transactionpb.AirdropType) *transactionpb.AirdropResponse {
	req := &transactionpb.AirdropRequest{
		AirdropType: airdropType,
		Owner:       p.parentAccount.ToProto(),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.Airdrop(p.ctx, req)
	require.NoError(t, err)
	return resp
}

func (p *phoneTestEnv) assertCanWithdrawToAccount(t *testing.T, target *common.Account, canWithdraw bool, asAccountType transactionpb.CanWithdrawToAccountResponse_AccountType) {
	resp, err := p.client.CanWithdrawToAccount(p.ctx, &transactionpb.CanWithdrawToAccountRequest{
		Account: target.ToProto(),
	})
	require.NoError(t, err)
	assert.Equal(t, canWithdraw, resp.IsValidPaymentDestination)
	assert.Equal(t, asAccountType, resp.AccountType)
}

func (p *phoneTestEnv) checkWithServerForAnyOtherPrivacyUpgrades(t *testing.T) bool {
	req := &transactionpb.GetPrioritizedIntentsForPrivacyUpgradeRequest{
		Owner: p.parentAccount.ToProto(),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	if resp.Result == transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND {
		return false
	}

	require.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)

	p.validateUpgradeableIntents(t, resp.Items)

	return true
}

func (p *phoneTestEnv) validateUpgradeableIntents(t *testing.T, upgradeableIntents []*transactionpb.UpgradeableIntent) {
	for _, intent := range upgradeableIntents {
		for _, action := range intent.Actions {
			// Find the locally stored upgrade metadata for the action server wants
			// us to upgrade. Real clients probably won't have this benefit.
			var txnToUpgrade *privacyUpgradeMetadata
			for _, upgradeMetadata := range p.txnsToUpgrade {
				if upgradeMetadata.intentId == base58.Encode(intent.Id.Value) && upgradeMetadata.actionId == action.ActionId {
					txnToUpgrade = &upgradeMetadata
					break
				}
			}
			require.NotNil(t, txnToUpgrade)

			var txn solana.Transaction
			require.NoError(t, txn.Unmarshal(action.TransactionBlob.Value))

			// Validate server provided the correct data. The proof for real
			// clients likely looks very different because they won't have the
			// local state.

			assert.Equal(t, txnToUpgrade.originalTransactionBlob, action.TransactionBlob.Value)
			assert.True(t, ed25519.Verify(txn.Message.Accounts[clientSignatureIndex], txn.Message.Marshal(), action.ClientSignature.Value))

			assert.Equal(t, txnToUpgrade.treasuryPool.PublicKey().ToBytes(), action.Treasury.Value)
			assert.Equal(t, txnToUpgrade.recentRoot, action.RecentRoot.Value)

			derivedAuthority := p.getDerivedAccount(t, action.SourceAccountType, action.SourceDerivationIndex)
			assert.Equal(t, txnToUpgrade.source.VaultOwner.PublicKey().ToBytes(), derivedAuthority.PublicKey().ToBytes())

			assert.Equal(t, txnToUpgrade.originalDestination.PublicKey().ToBytes(), action.OriginalDestination.Value)
			assert.Equal(t, txnToUpgrade.amount, action.OriginalAmount)
		}
	}
}

func (p *phoneTestEnv) getSendLimits(t *testing.T) map[string]*transactionpb.SendLimit {
	req := &transactionpb.GetLimitsRequest{
		Owner:         p.parentAccount.ToProto(),
		ConsumedSince: timestamppb.New(time.Now().Add(-24 * time.Hour)),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.GetLimits(p.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetLimitsResponse_OK, resp.Result)
	return resp.SendLimitsByCurrency
}

func (p *phoneTestEnv) getMicroPaymentLimits(t *testing.T) map[string]*transactionpb.MicroPaymentLimit {
	req := &transactionpb.GetLimitsRequest{
		Owner:         p.parentAccount.ToProto(),
		ConsumedSince: timestamppb.New(time.Now().Add(-24 * time.Hour)),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.GetLimits(p.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetLimitsResponse_OK, resp.Result)
	return resp.MicroPaymentLimitsByCurrency
}

func (p *phoneTestEnv) getBuyModuleLimits(t *testing.T) map[string]*transactionpb.BuyModuleLimit {
	req := &transactionpb.GetLimitsRequest{
		Owner:         p.parentAccount.ToProto(),
		ConsumedSince: timestamppb.New(time.Now().Add(-24 * time.Hour)),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.GetLimits(p.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetLimitsResponse_OK, resp.Result)
	return resp.BuyModuleLimitsByCurrency
}

func (p *phoneTestEnv) getDepositLimit(t *testing.T) *transactionpb.DepositLimit {
	req := &transactionpb.GetLimitsRequest{
		Owner:         p.parentAccount.ToProto(),
		ConsumedSince: timestamppb.New(time.Now().Add(-24 * time.Hour)),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.GetLimits(p.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetLimitsResponse_OK, resp.Result)
	return resp.DepositLimit
}

func (p *phoneTestEnv) declareFiatOnRampPurchase(t *testing.T, currency currency_lib.Code, amount float64, nonce uuid.UUID) transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_Result {
	req := &transactionpb.DeclareFiatOnrampPurchaseAttemptRequest{
		Owner: p.parentAccount.ToProto(),
		PurchaseAmount: &transactionpb.ExchangeDataWithoutRate{
			Currency:     string(currency),
			NativeAmount: amount,
		},
		Nonce: &commonpb.UUID{
			Value: nonce[:],
		},
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.DeclareFiatOnrampPurchaseAttempt(p.ctx, req)
	require.NoError(t, err)
	return resp.Result
}

func (p *phoneTestEnv) getPaymentHistory(t *testing.T) []*transactionpb.PaymentHistoryItem {
	req := &transactionpb.GetPaymentHistoryRequest{
		Owner: p.parentAccount.ToProto(),
	}
	req.Signature = p.signProtoMessage(t, req, req.Owner, false)

	resp, err := p.client.GetPaymentHistory(p.ctx, req)
	require.NoError(t, err)

	if len(resp.Items) == 0 {
		assert.Equal(t, transactionpb.GetPaymentHistoryResponse_NOT_FOUND, resp.Result)
	} else {
		assert.Equal(t, transactionpb.GetPaymentHistoryResponse_OK, resp.Result)
	}

	return resp.Items
}

func (p *phoneTestEnv) getCloseEmptyAccountTransactionToSign(
	t *testing.T,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.CloseEmptyAccountAction,
	intentMetadata *transactionpb.Metadata,
) txnToSign {
	dataVersion := timelock_token_v1.DataVersion1
	switch intentMetadata.Type.(type) {
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		dataVersion = timelock_token_v1.DataVersionLegacy
	}

	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	require.NoError(t, err)

	txn, err := transaction_util.MakeCloseEmptyAccountTransaction(
		nonce,
		bh,
		timelockAccounts,
	)
	require.NoError(t, err)

	return txnToSign{txn, authority}
}

func (p *phoneTestEnv) getCloseDormantAccountTransactionToSign(
	t *testing.T,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.CloseDormantAccountAction,
) txnToSign {
	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	destination, err := common.NewAccountFromProto(action.Destination)
	require.NoError(t, err)

	txn, err := transaction_util.MakeCloseAccountWithBalanceTransaction(
		nonce,
		bh,
		timelockAccounts,
		destination,
		nil,
	)
	require.NoError(t, err)

	return txnToSign{txn, authority}
}

func (p *phoneTestEnv) getNoPrivacyTransferTransactionToSign(
	t *testing.T,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.NoPrivacyTransferAction,
	intentMetadata *transactionpb.Metadata,
) txnToSign {
	dataVersion := timelock_token_v1.DataVersion1
	switch intentMetadata.Type.(type) {
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		dataVersion = timelock_token_v1.DataVersionLegacy
	}

	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	require.NoError(t, err)

	destination, err := common.NewAccountFromProto(action.Destination)
	require.NoError(t, err)

	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		nonce,
		bh,
		timelockAccounts,
		destination,
		action.Amount,
	)
	require.NoError(t, err)

	return txnToSign{txn, authority}
}

func (p *phoneTestEnv) getNoPrivacyWithdrawTransactionToSign(
	t *testing.T,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.NoPrivacyWithdrawAction,
	intentMetadata *transactionpb.Metadata,
) txnToSign {
	var additionalMemo *string
	dataVersion := timelock_token_v1.DataVersion1
	switch typed := intentMetadata.Type.(type) {
	case *transactionpb.Metadata_SendPrivatePayment:
		if typed.SendPrivatePayment.TippedUser != nil {
			tipMemo, err := transaction_util.GetTipMemoValue(typed.SendPrivatePayment.TippedUser.Platform, typed.SendPrivatePayment.TippedUser.Username)
			require.NoError(t, err)
			additionalMemo = &tipMemo
		}
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		dataVersion = timelock_token_v1.DataVersionLegacy
	}

	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	require.NoError(t, err)

	destination, err := common.NewAccountFromProto(action.Destination)
	require.NoError(t, err)

	txn, err := transaction_util.MakeCloseAccountWithBalanceTransaction(
		nonce,
		bh,
		timelockAccounts,
		destination,
		additionalMemo,
	)
	require.NoError(t, err)

	return txnToSign{txn, authority}
}

func (p *phoneTestEnv) getTemporaryPrivacyTransferTransactionToSign(
	t *testing.T,
	intentId string,
	actionId uint32,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.TemporaryPrivacyTransferAction,
	serverParameter *transactionpb.TemporaryPrivacyTransferServerParameter,
) (txnToSign, privacyUpgradeMetadata) {
	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	treasuryPool, err := common.NewAccountFromProto(serverParameter.Treasury)
	require.NoError(t, err)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	destination, err := common.NewAccountFromProto(action.Destination)
	require.NoError(t, err)

	transcript := getTransript(
		intentId,
		actionId,
		timelockAccounts.Vault,
		destination,
		action.Amount,
	)

	commitmentAddress, _, err := splitter_token.GetCommitmentStateAddress(&splitter_token.GetCommitmentStateAddressArgs{
		Pool:        treasuryPool.PublicKey().ToBytes(),
		RecentRoot:  serverParameter.RecentRoot.Value,
		Transcript:  transcript,
		Destination: destination.PublicKey().ToBytes(),
		Amount:      action.Amount,
	})
	require.NoError(t, err)
	commitment, err := common.NewAccountFromPublicKeyBytes(commitmentAddress)
	require.NoError(t, err)

	commitmentVaultAddress, _, err := splitter_token.GetCommitmentVaultAddress(&splitter_token.GetCommitmentVaultAddressArgs{
		Pool:       treasuryPool.PublicKey().ToBytes(),
		Commitment: commitment.PublicKey().ToBytes(),
	})
	require.NoError(t, err)
	commitmentVault, err := common.NewAccountFromPublicKeyBytes(commitmentVaultAddress)
	require.NoError(t, err)

	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		nonce,
		bh,
		timelockAccounts,
		commitmentVault,
		action.Amount,
	)
	require.NoError(t, err)

	upgradeMetadata := privacyUpgradeMetadata{
		intentId: intentId,
		actionId: actionId,

		source:              timelockAccounts,
		treasuryPool:        treasuryPool,
		recentRoot:          serverParameter.RecentRoot.Value,
		originalCommitment:  commitment,
		originalDestination: destination,
		amount:              action.Amount,

		originalTransactionBlob: txn.Marshal(),

		nonce: nonce,
		bh:    bh,
	}

	return txnToSign{txn, authority}, upgradeMetadata
}

func (p *phoneTestEnv) getTemporaryPrivacyExchangeTransactionToSign(
	t *testing.T,
	intentId string,
	actionId uint32,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.TemporaryPrivacyExchangeAction,
	serverParameter *transactionpb.TemporaryPrivacyExchangeServerParameter,
) (txnToSign, privacyUpgradeMetadata) {
	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	treasuryPool, err := common.NewAccountFromProto(serverParameter.Treasury)
	require.NoError(t, err)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	destination, err := common.NewAccountFromProto(action.Destination)
	require.NoError(t, err)

	transcript := getTransript(
		intentId,
		actionId,
		timelockAccounts.Vault,
		destination,
		action.Amount,
	)

	commitmentAddress, _, err := splitter_token.GetCommitmentStateAddress(&splitter_token.GetCommitmentStateAddressArgs{
		Pool:        treasuryPool.PublicKey().ToBytes(),
		RecentRoot:  serverParameter.RecentRoot.Value,
		Transcript:  transcript,
		Destination: destination.PublicKey().ToBytes(),
		Amount:      action.Amount,
	})
	require.NoError(t, err)
	commitment, err := common.NewAccountFromPublicKeyBytes(commitmentAddress)
	require.NoError(t, err)

	commitmentVaultAddress, _, err := splitter_token.GetCommitmentVaultAddress(&splitter_token.GetCommitmentVaultAddressArgs{
		Pool:       treasuryPool.PublicKey().ToBytes(),
		Commitment: commitment.PublicKey().ToBytes(),
	})
	require.NoError(t, err)
	commitmentVault, err := common.NewAccountFromPublicKeyBytes(commitmentVaultAddress)
	require.NoError(t, err)

	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		nonce,
		bh,
		timelockAccounts,
		commitmentVault,
		action.Amount,
	)
	require.NoError(t, err)

	upgradeMetadata := privacyUpgradeMetadata{
		intentId: intentId,
		actionId: actionId,

		source:              timelockAccounts,
		treasuryPool:        treasuryPool,
		recentRoot:          serverParameter.RecentRoot.Value,
		originalCommitment:  commitment,
		originalDestination: destination,
		amount:              action.Amount,

		originalTransactionBlob: txn.Marshal(),

		nonce: nonce,
		bh:    bh,
	}

	return txnToSign{txn, authority}, upgradeMetadata
}

func (p *phoneTestEnv) getPermanentPrivacyTransactionToSign(
	t *testing.T,
	intentId string,
	action *transactionpb.PermanentPrivacyUpgradeAction,
	serverParameter *transactionpb.PermanentPrivacyUpgradeServerParameter,
) txnToSign {
	// Get metadata about the transaction we're about to upgrade. This is required
	// for verifying proofs and constructing the upgraded transaction.
	var upgradeMetadata *privacyUpgradeMetadata
	for _, txnToUpgrade := range p.txnsToUpgrade {
		if txnToUpgrade.intentId == intentId && txnToUpgrade.actionId == action.ActionId {
			upgradeMetadata = &txnToUpgrade
			break
		}
	}
	require.NotNil(t, upgradeMetadata)

	// Prove that the provided merkle root happened after the original commitment
	proof := make([]merkletree.Hash, len(serverParameter.MerkleProof))
	for i, hash := range serverParameter.MerkleProof {
		proof[i] = hash.Value
	}
	require.True(t, merkletree.Verify(proof, serverParameter.MerkleRoot.Value, upgradeMetadata.originalCommitment.PublicKey().ToBytes()))

	// Prove that the new commitment used the merkle root as its recent root, which
	// implies it must have happened after the original commitment
	expectedNewCommitmentAddress, _, err := splitter_token.GetCommitmentStateAddress(
		&splitter_token.GetCommitmentStateAddressArgs{
			Pool:        upgradeMetadata.treasuryPool.PublicKey().ToBytes(),
			RecentRoot:  serverParameter.MerkleRoot.Value,
			Transcript:  serverParameter.NewCommitmentTranscript.Value,
			Destination: serverParameter.NewCommitmentDestination.Value,
			Amount:      serverParameter.NewCommitmentAmount,
		},
	)
	require.NoError(t, err)
	require.EqualValues(t, expectedNewCommitmentAddress, serverParameter.NewCommitment.Value)

	// We've now fully proven that it's safe to redirect funds from the original
	// commitment to the new commitment.

	newCommitment, err := common.NewAccountFromPublicKeyBytes(expectedNewCommitmentAddress)
	require.NoError(t, err)

	newCommitmentVaultAddress, _, err := splitter_token.GetCommitmentVaultAddress(&splitter_token.GetCommitmentVaultAddressArgs{
		Pool:       upgradeMetadata.treasuryPool.PublicKey().ToBytes(),
		Commitment: newCommitment.PublicKey().ToBytes(),
	})
	require.NoError(t, err)
	newCommitmentVault, err := common.NewAccountFromPublicKeyBytes(newCommitmentVaultAddress)
	require.NoError(t, err)

	// Notice the only difference in the constructed transaction is the destination.
	// Everything else is inferred from the transaction being upgraded.
	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		upgradeMetadata.nonce,
		upgradeMetadata.bh,
		upgradeMetadata.source,
		newCommitmentVault,
		upgradeMetadata.amount,
	)
	require.NoError(t, err)

	return txnToSign{txn, upgradeMetadata.source.VaultOwner}
}

func (p *phoneTestEnv) getFeePaymentTransactionToSign(
	t *testing.T,
	nonceMetadata *transactionpb.NoncedTransactionMetadata,
	action *transactionpb.FeePaymentAction,
	serverParameter *transactionpb.FeePaymentServerParameter,
) txnToSign {
	nonce, err := common.NewAccountFromProto(nonceMetadata.Nonce)
	require.NoError(t, err)

	var bh solana.Blockhash
	copy(bh[:], nonceMetadata.Blockhash.Value)

	authority, err := common.NewAccountFromProto(action.Authority)
	require.NoError(t, err)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	var destination *common.Account
	if action.Type == transactionpb.FeePaymentAction_CODE {
		require.NotNil(t, serverParameter.CodeDestination)
		destination, err = common.NewAccountFromProto(serverParameter.CodeDestination)
		require.NoError(t, err)
	} else {
		assert.Nil(t, serverParameter.CodeDestination)
		destination, err = common.NewAccountFromProto(action.Destination)
		require.NoError(t, err)
	}

	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		nonce,
		bh,
		timelockAccounts,
		destination,
		action.Amount,
	)
	require.NoError(t, err)

	return txnToSign{txn, authority}
}

func (p *phoneTestEnv) signProtoMessage(t *testing.T, msg proto.Message, protoAccount *commonpb.SolanaAccountId, simulateInvalidSignature bool) *commonpb.Signature {
	signer := p.getSigner(t, protoAccount.Value)
	return p.signProtoMessageWithSigner(t, msg, signer, simulateInvalidSignature)
}

func (p *phoneTestEnv) signProtoMessageWithSigner(t *testing.T, msg proto.Message, signer *common.Account, simulateInvalidSignature bool) *commonpb.Signature {
	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err)

	if simulateInvalidSignature {
		signer = testutil.NewRandomAccount(t)
	}

	signature, err := signer.Sign(msgBytes)
	require.NoError(t, err)

	return &commonpb.Signature{
		Value: signature,
	}
}

func (p *phoneTestEnv) getSigner(t *testing.T, publicKey []byte) *common.Account {
	if bytes.Equal(p.parentAccount.PublicKey().ToBytes(), publicKey) {
		return p.parentAccount
	}

	for _, derivedAccount := range p.allDerivedAccounts {
		if bytes.Equal(derivedAccount.value.PublicKey().ToBytes(), publicKey) {
			return derivedAccount.value
		}
	}

	for _, giftAccount := range p.allGiftCardAccounts {
		if bytes.Equal(giftAccount.PublicKey().ToBytes(), publicKey) {
			return giftAccount
		}
	}

	require.Fail(t, "signer not found")
	return nil
}

func (p *phoneTestEnv) resetConfig() {
	p.conf = phoneConf{}
}

func (p *phoneTestEnv) reset(t *testing.T) {
	p.allDerivedAccounts = make([]derivedAccount, 0)
	p.currentTempIncomingIndex = 0
	p.currentTempOutgoingIndex = 0
	p.txnsToUpgrade = make([]privacyUpgradeMetadata, 0)

	p.resetConfig()

	p.parentAccount = testutil.NewRandomAccount(t)

	for _, accountType := range accountTypesToOpen {
		if accountType == commonpb.AccountType_PRIMARY {
			continue
		}

		derived := derivedAccount{
			accountType: accountType,
			index:       0,
			value:       testutil.NewRandomAccount(t),
		}
		p.allDerivedAccounts = append(p.allDerivedAccounts, derived)
		p.currentDerivedAccounts[accountType] = derived.value
	}
}

func (p *phoneTestEnv) getTimelockVault(t *testing.T, accountType commonpb.AccountType, index uint64) *common.Account {
	return getTimelockVault(t, p.getAuthority(t, accountType, index))
}

func (p *phoneTestEnv) getAuthority(t *testing.T, accountType commonpb.AccountType, index uint64) *common.Account {
	if accountType == commonpb.AccountType_RELATIONSHIP {
		require.Fail(t, "relationship account not supported in this function")
	}

	if accountType == commonpb.AccountType_PRIMARY {
		return p.parentAccount
	}
	return p.getDerivedAccount(t, accountType, index)
}

func (p *phoneTestEnv) getAuthorityForLatestAccount(t *testing.T, accountType commonpb.AccountType) (*common.Account, uint64) {
	if accountType == commonpb.AccountType_RELATIONSHIP {
		require.Fail(t, "relationship account not supported in this function")
	}

	if accountType == commonpb.AccountType_PRIMARY {
		return p.parentAccount, 0
	}

	var index uint64
	if accountType == commonpb.AccountType_TEMPORARY_INCOMING {
		index = p.currentTempIncomingIndex
	} else if accountType == commonpb.AccountType_TEMPORARY_OUTGOING {
		index = p.currentTempOutgoingIndex
	}

	return p.getDerivedAccount(t, accountType, index), index
}

func (p *phoneTestEnv) getAuthorityForRelationshipAccount(t *testing.T, relationship string) *common.Account {
	for i := len(p.allDerivedAccounts) - 1; i >= 0; i-- {
		if p.allDerivedAccounts[i].accountType == commonpb.AccountType_RELATIONSHIP && *p.allDerivedAccounts[i].relationshipTo == relationship {
			return p.allDerivedAccounts[i].value
		}
	}

	require.Fail(t, "relationship account not found")

	return nil
}

func (p *phoneTestEnv) getDerivedAccount(t *testing.T, accountType commonpb.AccountType, index uint64) *common.Account {
	if accountType == commonpb.AccountType_RELATIONSHIP {
		require.Fail(t, "relationship account not supported in this function")
	}

	for i := len(p.allDerivedAccounts) - 1; i >= 0; i-- {
		if p.allDerivedAccounts[i].accountType == accountType && p.allDerivedAccounts[i].index == index {
			return p.allDerivedAccounts[i].value
		}
	}

	require.Fail(t, "derived account not found")

	return nil
}

func (p *phoneTestEnv) assertAirdropCount(t *testing.T, expected int) {
	history := p.getPaymentHistory(t)

	var actual int
	for _, historyItem := range history {
		if historyItem.IsAirdrop {
			actual += 1
		}
	}
	assert.Equal(t, expected, actual)
}

func (m submitIntentCallMetadata) requireSuccess(t *testing.T) {
	require.NoError(t, m.err)
	if m.resp.GetError() != nil {
		require.Fail(t, m.resp.GetError().Code.String(), m.resp.GetError().String())
	}
}

func (m submitIntentCallMetadata) assertDeniedResponse(t *testing.T, message string) {
	require.NoError(t, m.err)

	require.NotNil(t, m.resp.GetError())
	assert.Equal(t, transactionpb.SubmitIntentResponse_Error_DENIED, m.resp.GetError().Code)

	require.Len(t, m.resp.GetError().GetErrorDetails(), 1)
	errorDetails := m.resp.GetError().GetErrorDetails()[0]
	require.NotNil(t, errorDetails.GetDenied())
	assert.True(t, strings.Contains(errorDetails.GetDenied().Reason, message))
}

func (m submitIntentCallMetadata) assertInvalidIntentResponse(t *testing.T, message string) {
	require.NoError(t, m.err)

	require.NotNil(t, m.resp.GetError())
	assert.Equal(t, transactionpb.SubmitIntentResponse_Error_INVALID_INTENT, m.resp.GetError().Code)

	require.Len(t, m.resp.GetError().GetErrorDetails(), 1)
	errorDetails := m.resp.GetError().GetErrorDetails()[0]
	require.NotNil(t, errorDetails.GetReasonString())
	assert.True(t, strings.Contains(errorDetails.GetReasonString().Reason, message))
}

func (m submitIntentCallMetadata) assertStaleStateResponse(t *testing.T, message string) {
	require.NoError(t, m.err)

	require.NotNil(t, m.resp.GetError())
	assert.Equal(t, transactionpb.SubmitIntentResponse_Error_STALE_STATE, m.resp.GetError().Code)

	require.Len(t, m.resp.GetError().GetErrorDetails(), 1)
	errorDetails := m.resp.GetError().GetErrorDetails()[0]
	require.NotNil(t, errorDetails.GetReasonString())
	assert.True(t, strings.Contains(errorDetails.GetReasonString().Reason, message))
}

func (m submitIntentCallMetadata) assertSignatureErrorResponse(t *testing.T, message string) {
	require.NoError(t, m.err)

	require.NotNil(t, m.resp.GetError())
	assert.Equal(t, transactionpb.SubmitIntentResponse_Error_SIGNATURE_ERROR, m.resp.GetError().Code)

	require.Len(t, m.resp.GetError().GetErrorDetails(), 1)
	errorDetails := m.resp.GetError().GetErrorDetails()[0]
	require.NotNil(t, errorDetails.GetReasonString())
	assert.True(t, strings.Contains(errorDetails.GetReasonString().Reason, message))
}

func (m submitIntentCallMetadata) assertInvalidSignatureValueResponse(t *testing.T) {
	require.NoError(t, m.err)

	require.NotNil(t, m.resp.GetError())
	assert.Equal(t, transactionpb.SubmitIntentResponse_Error_SIGNATURE_ERROR, m.resp.GetError().Code)

	require.Len(t, m.resp.GetError().GetErrorDetails(), 1)
	errorDetails := m.resp.GetError().GetErrorDetails()[0]
	require.NotNil(t, errorDetails.GetInvalidSignature())

	var txn solana.Transaction
	require.NoError(t, txn.Unmarshal(errorDetails.GetInvalidSignature().ExpectedTransaction.Value))

	var emptySignature solana.Signature
	for _, sig := range txn.Signatures {
		assert.EqualValues(t, emptySignature, sig)
	}
}

func (m submitIntentCallMetadata) assertGrpcError(t *testing.T, code codes.Code) {
	assert.Nil(t, m.resp)

	if code == codes.DeadlineExceeded && m.err == io.EOF {
		return
	}
	testutil.AssertStatusErrorWithCode(t, m.err, code)
}

func (m submitIntentCallMetadata) assertGrpcErrorWithMessage(t *testing.T, code codes.Code, message string) {
	m.assertGrpcError(t, code)

	assert.True(t, strings.Contains(m.err.Error(), message))
}

func (m submitIntentCallMetadata) isError(t *testing.T) bool {
	return m.err != nil || m.resp.GetError() != nil
}

func getTimelockVault(t *testing.T, owner *common.Account) *common.Account {
	timelockAccounts, err := owner.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return timelockAccounts.Vault
}

func isSubmitIntentError(resp *transactionpb.SubmitIntentResponse, err error) bool {
	return err != nil || resp.GetError() != nil
}

func getProtoChatMessage(t *testing.T, record *chat_v1.Message) *chatpb.ChatMessage {
	var protoMessage chatpb.ChatMessage
	require.NoError(t, proto.Unmarshal(record.Data, &protoMessage))
	return &protoMessage
}
