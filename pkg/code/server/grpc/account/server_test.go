package account

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountpb "github.com/code-payments/code-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx        context.Context
	client     accountpb.AccountClient
	server     *server
	data       code_data.Provider
	subsidizer *common.Account
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = accountpb.NewAccountClient(conn)
	env.data = code_data.NewTestDataProvider()
	env.subsidizer = testutil.SetupRandomSubsidizer(t, env.data)

	s := NewAccountServer(env.data)
	env.server = s.(*server)

	serv.RegisterService(func(server *grpc.Server) {
		accountpb.RegisterAccountServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func TestIsCodeAccount_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	swapDerivedOwnerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.IsCodeAccountRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	resp, err := env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_NOT_FOUND, resp.Result)

	// Technically an invalid reality, but SubmitIntent guarantees all or no accounts
	// are opened, which allows IsCodeAccount to do lazy checking.
	setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)
	setupAccountRecords(t, env, ownerAccount, swapDerivedOwnerAccount, 0, commonpb.AccountType_SWAP)

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)
}

func TestIsCodeAccount_LegacyPrimary2022Migration_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.IsCodeAccountRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	resp, err := env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_NOT_FOUND, resp.Result)

	legacyAccountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_LEGACY_PRIMARY_2022)

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)

	setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)

	setupPrivacyMigration2022Intent(t, env, ownerAccount)

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)

	legacyAccountRecords.Timelock.VaultState = timelock_token_v1.StateClosed
	legacyAccountRecords.Timelock.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, legacyAccountRecords.Timelock))

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)
}

func TestIsCodeAccount_NotManagedByCode(t *testing.T) {
	for i := 0; i < 5; i++ {
		for _, unmanagedState := range []timelock_token_v1.TimelockState{
			timelock_token_v1.StateWaitingForTimeout,
			timelock_token_v1.StateUnlocked,
		} {
			env, cleanup := setup(t)
			defer cleanup()

			ownerAccount := testutil.NewRandomAccount(t)

			req := &accountpb.IsCodeAccountRequest{
				Owner: ownerAccount.ToProto(),
			}
			reqBytes, err := proto.Marshal(req)
			require.NoError(t, err)
			req.Signature = &commonpb.Signature{
				Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
			}

			resp, err := env.client.IsCodeAccount(env.ctx, req)
			require.NoError(t, err)
			assert.Equal(t, accountpb.IsCodeAccountResponse_NOT_FOUND, resp.Result)

			var allAccountRecords []*common.AccountRecords
			allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_LEGACY_PRIMARY_2022))
			allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY))
			allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, testutil.NewRandomAccount(t), 0, commonpb.AccountType_BUCKET_100_KIN))
			allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, testutil.NewRandomAccount(t), 0, commonpb.AccountType_TEMPORARY_INCOMING))
			allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, testutil.NewRandomAccount(t), 0, commonpb.AccountType_TEMPORARY_OUTGOING))

			resp, err = env.client.IsCodeAccount(env.ctx, req)
			require.NoError(t, err)
			assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)

			allAccountRecords[i].Timelock.VaultState = unmanagedState
			allAccountRecords[i].Timelock.Block += 1
			require.NoError(t, env.data.SaveTimelock(env.ctx, allAccountRecords[i].Timelock))

			resp, err = env.client.IsCodeAccount(env.ctx, req)
			require.NoError(t, err)
			assert.Equal(t, accountpb.IsCodeAccountResponse_UNLOCKED_TIMELOCK_ACCOUNT, resp.Result)
		}
	}
}

func TestGetTokenAccountInfos_UserAccounts_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	bucketDerivedOwner := testutil.NewRandomAccount(t)
	tempIncomingDerivedOwner := testutil.NewRandomAccount(t)
	relationship1DerivedOwner := testutil.NewRandomAccount(t)
	relationship2DerivedOwner := testutil.NewRandomAccount(t)
	swapDerivedOwner := testutil.NewRandomAccount(t)

	primaryAccountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)
	bucketAccountRecords := setupAccountRecords(t, env, ownerAccount, bucketDerivedOwner, 0, commonpb.AccountType_BUCKET_100_KIN)
	relationship1AccountRecords := setupAccountRecords(t, env, ownerAccount, relationship1DerivedOwner, 0, commonpb.AccountType_RELATIONSHIP)
	relationship2AccountRecords := setupAccountRecords(t, env, ownerAccount, relationship2DerivedOwner, 0, commonpb.AccountType_RELATIONSHIP)
	setupAccountRecords(t, env, ownerAccount, swapDerivedOwner, 0, commonpb.AccountType_SWAP)
	setupAccountRecords(t, env, ownerAccount, tempIncomingDerivedOwner, 2, commonpb.AccountType_TEMPORARY_INCOMING)
	setupCachedBalance(t, env, bucketAccountRecords, kin.ToQuarks(100))
	setupCachedBalance(t, env, primaryAccountRecords, kin.ToQuarks(42))
	setupCachedBalance(t, env, relationship1AccountRecords, kin.ToQuarks(999))
	setupCachedBalance(t, env, relationship2AccountRecords, kin.ToQuarks(5))

	otherOwnerAccount := testutil.NewRandomAccount(t)
	setupAccountRecords(t, env, otherOwnerAccount, otherOwnerAccount, 0, commonpb.AccountType_PRIMARY)
	setupAccountRecords(t, env, otherOwnerAccount, testutil.NewRandomAccount(t), 0, commonpb.AccountType_BUCKET_100_KIN)
	setupAccountRecords(t, env, otherOwnerAccount, testutil.NewRandomAccount(t), 2, commonpb.AccountType_TEMPORARY_INCOMING)

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 6)

	for _, authority := range []*common.Account{
		ownerAccount,
		bucketDerivedOwner,
		tempIncomingDerivedOwner,
		relationship1DerivedOwner,
		relationship2DerivedOwner,
		swapDerivedOwner,
	} {
		var tokenAccount *common.Account
		if authority.PublicKey().ToBase58() == swapDerivedOwner.PublicKey().ToBase58() {
			tokenAccount, err = authority.ToAssociatedTokenAccount(common.UsdcMintAccount)
			require.NoError(t, err)
		} else {
			timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
			require.NoError(t, err)
			tokenAccount = timelockAccounts.Vault
		}

		accountInfo, ok := resp.TokenAccountInfos[tokenAccount.PublicKey().ToBase58()]
		require.True(t, ok)

		assert.Equal(t, tokenAccount.PublicKey().ToBytes(), accountInfo.Address.Value)
		assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Owner.Value)
		assert.Equal(t, authority.PublicKey().ToBytes(), accountInfo.Authority.Value)

		switch authority.PublicKey().ToBase58() {
		case ownerAccount.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_PRIMARY, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, kin.ToQuarks(42), accountInfo.Balance)
		case bucketDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_BUCKET_100_KIN, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, kin.ToQuarks(100), accountInfo.Balance)
		case tempIncomingDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_TEMPORARY_INCOMING, accountInfo.AccountType)
			assert.EqualValues(t, 2, accountInfo.Index)
			assert.EqualValues(t, 0, accountInfo.Balance)
		case relationship1DerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_RELATIONSHIP, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, kin.ToQuarks(999), accountInfo.Balance)
			require.NotNil(t, accountInfo.Relationship)
			assert.Equal(t, *relationship1AccountRecords.General.RelationshipTo, accountInfo.Relationship.GetDomain().Value)
		case relationship2DerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_RELATIONSHIP, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, kin.ToQuarks(5), accountInfo.Balance)
			require.NotNil(t, accountInfo.Relationship)
			assert.Equal(t, *relationship2AccountRecords.General.RelationshipTo, accountInfo.Relationship.GetDomain().Value)
		case swapDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_SWAP, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, 0, accountInfo.Balance)
		default:
			require.Fail(t, "unexpected authority")
		}

		if accountInfo.AccountType != commonpb.AccountType_RELATIONSHIP {
			assert.Nil(t, accountInfo.Relationship)
		}

		if accountInfo.AccountType == commonpb.AccountType_SWAP {
			assert.Equal(t, accountpb.TokenAccountInfo_BALANCE_SOURCE_BLOCKCHAIN, accountInfo.BalanceSource)
			assert.Equal(t, accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE, accountInfo.ManagementState)
			assert.Equal(t, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_UNKNOWN, accountInfo.BlockchainState)
			assert.Equal(t, common.UsdcMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
		} else {
			assert.Equal(t, accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE, accountInfo.BalanceSource)
			assert.Equal(t, accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED, accountInfo.ManagementState)
			assert.Equal(t, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS, accountInfo.BlockchainState)
			assert.Equal(t, common.KinMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
		}

		assert.False(t, accountInfo.MustRotate)
		assert.Equal(t, accountpb.TokenAccountInfo_CLAIM_STATE_UNKNOWN, accountInfo.ClaimState)
		assert.Nil(t, accountInfo.OriginalExchangeData)
	}

	primaryAccountInfoRecord, err := env.data.GetLatestAccountInfoByOwnerAddressAndType(env.ctx, ownerAccount.PublicKey().ToBase58(), commonpb.AccountType_PRIMARY)
	require.NoError(t, err)
	assert.True(t, primaryAccountInfoRecord.RequiresDepositSync)
}

func TestGetTokenAccountInfos_RemoteSendGiftCard_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	// Test cases represent main iterations of a gift card account's state throughout
	// its lifecycle. All states beyond claimed status are not fully tested here and
	// are done elsewhere.
	for i, tc := range []struct {
		balance                  uint64
		timelockState            timelock_token_v1.TimelockState
		simulateClaimInCode      bool
		simulateAutoReturnInCode bool
		expectedBalanceSource    accountpb.TokenAccountInfo_BalanceSource
		expectedBlockchainState  accountpb.TokenAccountInfo_BlockchainState
		expectedManagementState  accountpb.TokenAccountInfo_ManagementState
		expectedClaimState       accountpb.TokenAccountInfo_ClaimState
	}{
		{
			balance:                  0,
			timelockState:            timelock_token_v1.StateLocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  0,
			timelockState:            timelock_token_v1.StateClosed,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  0,
			timelockState:            timelock_token_v1.StateUnlocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_UNKNOWN,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_UNKNOWN,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateLocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_NOT_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateClosed,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateLocked,
			simulateClaimInCode:      true,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateUnlocked,
			simulateClaimInCode:      true,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateClosed,
			simulateClaimInCode:      true,
			simulateAutoReturnInCode: false,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateLocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: true,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateUnlocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: true,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateClosed,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: true,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED,
		},
	} {
		phoneNumber := fmt.Sprintf("+1800555%d", i)
		ownerAccount := testutil.NewRandomAccount(t)
		timelockAccounts, err := ownerAccount.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
		require.NoError(t, err)

		req := &accountpb.GetTokenAccountInfosRequest{
			Owner: ownerAccount.ToProto(),
		}
		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		userIdentityRecord := &user_identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.PutUser(env.ctx, userIdentityRecord))

		accountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_REMOTE_SEND_GIFT_CARD)

		giftCardIssuedIntentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPrivatePayment,

			InitiatorOwnerAccount: testutil.NewRandomAccount(t).PrivateKey().ToBase58(),
			InitiatorPhoneNumber:  &phoneNumber,

			SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
				DestinationTokenAccount: accountRecords.General.TokenAccount,
				Quantity:                kin.ToQuarks(10),

				ExchangeCurrency: currency.CAD,
				ExchangeRate:     1.23,
				NativeAmount:     12.3,
				UsdMarketValue:   24.6,

				IsWithdrawal: false,
				IsRemoteSend: true,
			},

			State: intent.StatePending,
		}
		require.NoError(t, env.data.SaveIntent(env.ctx, giftCardIssuedIntentRecord))

		if tc.balance > 0 {
			setupCachedBalance(t, env, accountRecords, tc.balance)
		}

		autoReturnActionRecord := &action.Record{
			Intent:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPrivatePayment,

			ActionId:   0,
			ActionType: action.CloseDormantAccount,

			Source:      accountRecords.General.TokenAccount,
			Destination: pointer.String("primary"),
			Quantity:    nil,

			InitiatorPhoneNumber: &phoneNumber,

			State: action.StateUnknown,
		}
		if tc.simulateAutoReturnInCode {
			autoReturnActionRecord.State = action.StatePending
		}
		require.NoError(t, env.data.PutAllActions(env.ctx, autoReturnActionRecord))

		if tc.simulateClaimInCode {
			claimActionRecord := &action.Record{
				Intent:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
				IntentType: intent.ReceivePaymentsPublicly,

				ActionId:   0,
				ActionType: action.NoPrivacyWithdraw,

				Source:      accountRecords.General.TokenAccount,
				Destination: pointer.String("destination"),
				Quantity:    pointer.Uint64(tc.balance - 1), // Explicitly less than the actual balance

				InitiatorPhoneNumber: &phoneNumber,

				State: action.StatePending,
			}
			require.NoError(t, env.data.PutAllActions(env.ctx, claimActionRecord))
		}

		accountRecords.Timelock.VaultState = tc.timelockState
		accountRecords.Timelock.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))

		resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
		assert.Len(t, resp.TokenAccountInfos, 1)

		accountInfo, ok := resp.TokenAccountInfos[timelockAccounts.Vault.PublicKey().ToBase58()]
		require.True(t, ok)

		assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, accountInfo.AccountType)
		assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Owner.Value)
		assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Authority.Value)
		assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBytes(), accountInfo.Address.Value)
		assert.Equal(t, common.KinMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
		assert.EqualValues(t, 0, accountInfo.Index)

		assert.Equal(t, tc.expectedBalanceSource, accountInfo.BalanceSource)
		if tc.simulateClaimInCode || tc.simulateAutoReturnInCode || tc.expectedClaimState == accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED || tc.expectedClaimState == accountpb.TokenAccountInfo_CLAIM_STATE_EXPIRED {
			assert.EqualValues(t, 0, accountInfo.Balance)
		} else if tc.expectedBalanceSource == accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE {
			assert.EqualValues(t, tc.balance, accountInfo.Balance)
		} else {
			assert.EqualValues(t, 0, accountInfo.Balance)
		}

		assert.Equal(t, tc.expectedManagementState, accountInfo.ManagementState)
		assert.Equal(t, tc.expectedBlockchainState, accountInfo.BlockchainState)
		assert.Equal(t, tc.expectedClaimState, accountInfo.ClaimState)

		require.NotNil(t, accountInfo.OriginalExchangeData)
		assert.EqualValues(t, giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.ExchangeCurrency, accountInfo.OriginalExchangeData.Currency)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.ExchangeRate, accountInfo.OriginalExchangeData.ExchangeRate)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.NativeAmount, accountInfo.OriginalExchangeData.NativeAmount)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.Quantity, accountInfo.OriginalExchangeData.Quarks)

		assert.False(t, accountInfo.MustRotate)

		assert.Nil(t, accountInfo.Relationship)

		accountInfoRecord, err := env.data.GetLatestAccountInfoByOwnerAddressAndType(env.ctx, ownerAccount.PublicKey().ToBase58(), commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
		require.NoError(t, err)
		assert.False(t, accountInfoRecord.RequiresDepositSync)
	}
}

func TestGetTokenAccountInfos_BlockchainState(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	for _, tc := range []struct {
		timelockState timelock_token_v1.TimelockState
		expected      accountpb.TokenAccountInfo_BlockchainState
	}{
		{timelock_token_v1.StateUnknown, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST},
		{timelock_token_v1.StateUnlocked, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS},
		{timelock_token_v1.StateWaitingForTimeout, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS},
		{timelock_token_v1.StateLocked, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS},
		{timelock_token_v1.StateClosed, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST},
	} {
		ownerAccount := testutil.NewRandomAccount(t)

		req := &accountpb.GetTokenAccountInfosRequest{
			Owner: ownerAccount.ToProto(),
		}
		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		accountRecords := getDefaultTestAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)
		accountRecords.Timelock.VaultState = tc.timelockState
		accountRecords.Timelock.Block += 1
		require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountRecords.General))
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))

		resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
		assert.Len(t, resp.TokenAccountInfos, 1)

		accountInfo, ok := resp.TokenAccountInfos[accountRecords.Timelock.VaultAddress]
		require.True(t, ok)
		assert.Equal(t, tc.expected, accountInfo.BlockchainState)
	}
}

func TestGetTokenAccountInfos_ManagementState(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	for _, tc := range []struct {
		timelockState  timelock_token_v1.TimelockState
		block          uint64
		timeAuthority  *common.Account
		closeAuthority *common.Account
		expected       accountpb.TokenAccountInfo_ManagementState
	}{
		{
			timelockState:  timelock_token_v1.StateUnknown,
			block:          0,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
		},
		{timelockState: timelock_token_v1.StateUnknown,
			block:          1,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNKNOWN,
		},
		{timelockState: timelock_token_v1.StateUnlocked,
			block:          2,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
		},
		{
			timelockState:  timelock_token_v1.StateWaitingForTimeout,
			block:          3,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKING,
		},
		{
			timelockState:  timelock_token_v1.StateLocked,
			block:          4,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
		},
		{
			timelockState:  timelock_token_v1.StateClosed,
			block:          5,
			timeAuthority:  env.subsidizer,
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
		},
		{
			timelockState:  timelock_token_v1.StateLocked,
			block:          6,
			timeAuthority:  testutil.NewRandomAccount(t),
			closeAuthority: env.subsidizer,
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE,
		},
		{
			timelockState:  timelock_token_v1.StateLocked,
			block:          7,
			timeAuthority:  env.subsidizer,
			closeAuthority: testutil.NewRandomAccount(t),
			expected:       accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE,
		},
	} {
		ownerAccount := testutil.NewRandomAccount(t)

		req := &accountpb.GetTokenAccountInfosRequest{
			Owner: ownerAccount.ToProto(),
		}
		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		accountRecords := getDefaultTestAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)
		accountRecords.Timelock.VaultState = tc.timelockState
		accountRecords.Timelock.Block = tc.block
		accountRecords.Timelock.TimeAuthority = tc.timeAuthority.PublicKey().ToBase58()
		accountRecords.Timelock.CloseAuthority = tc.closeAuthority.PublicKey().ToBase58()
		require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountRecords.General))
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))

		resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
		assert.Len(t, resp.TokenAccountInfos, 1)

		accountInfo, ok := resp.TokenAccountInfos[accountRecords.Timelock.VaultAddress]
		require.True(t, ok)
		assert.Equal(t, tc.expected, accountInfo.ManagementState)
	}
}

func TestGetTokenAccountInfos_TempIncomingAccountRotation_AtLeastOnePayment(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	tempIncomingDerivedOwner := testutil.NewRandomAccount(t)
	accountRecords := setupAccountRecords(t, env, ownerAccount, tempIncomingDerivedOwner, 2, commonpb.AccountType_TEMPORARY_INCOMING)

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	require.Len(t, resp.TokenAccountInfos, 1)

	accountInfo, ok := resp.TokenAccountInfos[accountRecords.General.TokenAccount]
	require.True(t, ok)
	assert.False(t, accountInfo.MustRotate)

	for i := 0; i < 3; i++ {
		quantity := uint64(1)
		actionRecord := &action.Record{
			Intent:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType:  intent.SendPrivatePayment,
			ActionType:  action.NoPrivacyWithdraw,
			Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Destination: &accountRecords.General.TokenAccount,
			Quantity:    &quantity,
			State:       action.StatePending,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, env.data.PutAllActions(env.ctx, actionRecord))

		resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
		require.Len(t, resp.TokenAccountInfos, 1)

		accountInfo, ok := resp.TokenAccountInfos[accountRecords.General.TokenAccount]
		require.True(t, ok)
		assert.True(t, accountInfo.MustRotate)
	}
}

func TestGetTokenAccountInfos_NoTokenAccounts(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_NOT_FOUND, resp.Result)
	assert.Empty(t, resp.TokenAccountInfos)
}

func TestGetTokenAccountInfos_LegacyPrimary2022Migration_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	accountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_LEGACY_PRIMARY_2022)
	setupCachedBalance(t, env, accountRecords, kin.ToQuarks(123))

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 1)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
	require.NoError(t, err)

	accountInfo, ok := resp.TokenAccountInfos[timelockAccounts.Vault.PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, commonpb.AccountType_LEGACY_PRIMARY_2022, accountInfo.AccountType)
	assert.EqualValues(t, 0, accountInfo.Index)
	assert.Equal(t, timelockAccounts.Vault.PublicKey().ToBytes(), accountInfo.Address.Value)
	assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Owner.Value)
	assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Authority.Value)
	assert.Equal(t, common.KinMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
	assert.Equal(t, accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE, accountInfo.BalanceSource)
	assert.EqualValues(t, kin.ToQuarks(123), accountInfo.Balance)
	assert.Equal(t, accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED, accountInfo.ManagementState)
	assert.Equal(t, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS, accountInfo.BlockchainState)
	assert.False(t, accountInfo.MustRotate)
}

func TestGetTokenAccountInfos_LegacyPrimary2022Migration_IntentSubmitted(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	accountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_LEGACY_PRIMARY_2022)
	setupCachedBalance(t, env, accountRecords, kin.ToQuarks(123))

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 1)

	setupPrivacyMigration2022Intent(t, env, ownerAccount)

	resp, err = env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_NOT_FOUND, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 0)
}

func TestGetTokenAccountInfos_LegacyPrimary2022Migration_AccountClosed(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	accountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_LEGACY_PRIMARY_2022)
	setupCachedBalance(t, env, accountRecords, kin.ToQuarks(123))

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 1)

	accountRecords.Timelock.VaultState = timelock_token_v1.StateClosed
	accountRecords.Timelock.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))

	resp, err = env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_NOT_FOUND, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 0)
}

func TestLinkAdditionalAccounts_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	swapAuthorityAccount := testutil.NewRandomAccount(t)

	setupOpenAccountsIntent(t, env, ownerAccount)

	req := &accountpb.LinkAdditionalAccountsRequest{
		Owner:         ownerAccount.ToProto(),
		SwapAuthority: swapAuthorityAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signatures := []*commonpb.Signature{
		{Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes)},
		{Value: ed25519.Sign(swapAuthorityAccount.PrivateKey().ToBytes(), reqBytes)},
	}
	req.Signatures = signatures

	resp, err := env.client.LinkAdditionalAccounts(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.LinkAdditionalAccountsResponse_OK, resp.Result)

	expectedSwapUsdcAta, err := swapAuthorityAccount.ToAssociatedTokenAccount(common.UsdcMintAccount)
	require.NoError(t, err)

	accountInfoRecord, err := env.data.GetAccountInfoByAuthorityAddress(env.ctx, swapAuthorityAccount.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, ownerAccount.PublicKey().ToBase58(), accountInfoRecord.OwnerAccount)
	assert.Equal(t, swapAuthorityAccount.PublicKey().ToBase58(), accountInfoRecord.AuthorityAccount)
	assert.Equal(t, expectedSwapUsdcAta.PublicKey().ToBase58(), accountInfoRecord.TokenAccount)
	assert.Equal(t, common.UsdcMintAccount.PublicKey().ToBase58(), accountInfoRecord.MintAccount)
	assert.Equal(t, commonpb.AccountType_SWAP, accountInfoRecord.AccountType)
	assert.EqualValues(t, 0, accountInfoRecord.Index)

	resp, err = env.client.LinkAdditionalAccounts(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.LinkAdditionalAccountsResponse_OK, resp.Result)
}

func TestLinkAdditionalAccounts_UserAccountsNotOpened(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	swapAuthorityAccount := testutil.NewRandomAccount(t)

	req := &accountpb.LinkAdditionalAccountsRequest{
		Owner:         ownerAccount.ToProto(),
		SwapAuthority: swapAuthorityAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signatures := []*commonpb.Signature{
		{Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes)},
		{Value: ed25519.Sign(swapAuthorityAccount.PrivateKey().ToBytes(), reqBytes)},
	}
	req.Signatures = signatures

	resp, err := env.client.LinkAdditionalAccounts(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.LinkAdditionalAccountsResponse_DENIED, resp.Result)

	_, err = env.data.GetAccountInfoByAuthorityAddress(env.ctx, swapAuthorityAccount.PublicKey().ToBase58())
	assert.Equal(t, account.ErrAccountInfoNotFound, err)
}

func TestLinkAdditionalAccounts_InvalidSwapAuthority(t *testing.T) {
	ownerAccount := testutil.NewRandomAccount(t)
	swapAuthorityAccount := testutil.NewRandomAccount(t)
	expectedSwapUsdcAta, err := swapAuthorityAccount.ToAssociatedTokenAccount(common.UsdcMintAccount)
	require.NoError(t, err)

	for _, tc := range []struct {
		swapAuthorityAccount *common.Account
		setup                func(t *testing.T, env testEnv)
	}{
		{
			ownerAccount,
			func(t *testing.T, env testEnv) {},
		},
		{
			swapAuthorityAccount,
			func(t *testing.T, env testEnv) {
				require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
					OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
					AuthorityAccount: swapAuthorityAccount.PublicKey().ToBase58(),
					TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
					MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
					AccountType:      commonpb.AccountType_BUCKET_100_000_KIN,
				}))
			},
		},
		{
			swapAuthorityAccount,
			func(t *testing.T, env testEnv) {
				require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
					OwnerAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
					AuthorityAccount: swapAuthorityAccount.PublicKey().ToBase58(),
					TokenAccount:     expectedSwapUsdcAta.PublicKey().ToBase58(),
					MintAccount:      common.UsdcMintAccount.PublicKey().ToBase58(),
					AccountType:      commonpb.AccountType_SWAP,
				}))
			},
		},
	} {
		env, cleanup := setup(t)
		defer cleanup()

		swapAuthorityAccount := tc.swapAuthorityAccount

		setupOpenAccountsIntent(t, env, ownerAccount)
		tc.setup(t, env)

		req := &accountpb.LinkAdditionalAccountsRequest{
			Owner:         ownerAccount.ToProto(),
			SwapAuthority: swapAuthorityAccount.ToProto(),
		}
		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)
		signatures := []*commonpb.Signature{
			{Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes)},
			{Value: ed25519.Sign(swapAuthorityAccount.PrivateKey().ToBytes(), reqBytes)},
		}
		req.Signatures = signatures

		resp, err := env.client.LinkAdditionalAccounts(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.LinkAdditionalAccountsResponse_INVALID_ACCOUNT, resp.Result)

		accountInfoRecord, err := env.data.GetAccountInfoByAuthorityAddress(env.ctx, swapAuthorityAccount.PublicKey().ToBase58())
		if err == nil {
			// If there's a record, then at least one of these conditions must be false
			isSwapAccount := accountInfoRecord.AccountType == commonpb.AccountType_SWAP
			isSametOwner := accountInfoRecord.OwnerAccount == ownerAccount.PublicKey().ToBase58()
			isExpectedTokenAccount := accountInfoRecord.TokenAccount == expectedSwapUsdcAta.PublicKey().ToBase58()
			assert.True(t, !isSwapAccount || !isSametOwner || !isExpectedTokenAccount)
		} else if err != account.ErrAccountInfoNotFound {
			require.NoError(t, err)
		}
	}
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	swapAuthorityAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	isCodeAccountReq := &accountpb.IsCodeAccountRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(isCodeAccountReq)
	require.NoError(t, err)
	isCodeAccountReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	_, err = env.client.IsCodeAccount(env.ctx, isCodeAccountReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	getTokenAccountInfosReq := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err = proto.Marshal(getTokenAccountInfosReq)
	require.NoError(t, err)
	getTokenAccountInfosReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	_, err = env.client.GetTokenAccountInfos(env.ctx, getTokenAccountInfosReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	for i := 0; i < 2; i++ {
		linkReq := &accountpb.LinkAdditionalAccountsRequest{
			Owner:         ownerAccount.ToProto(),
			SwapAuthority: swapAuthorityAccount.ToProto(),
		}
		reqBytes, err = proto.Marshal(linkReq)
		require.NoError(t, err)
		signatures := []*commonpb.Signature{
			{Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes)},
			{Value: ed25519.Sign(swapAuthorityAccount.PrivateKey().ToBytes(), reqBytes)},
		}
		signatures[i].Value = ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes)
		linkReq.Signatures = signatures

		_, err = env.client.LinkAdditionalAccounts(env.ctx, linkReq)
		testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
	}
}

func setupAccountRecords(t *testing.T, env testEnv, ownerAccount, authorityAccount *common.Account, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	accountRecords := getDefaultTestAccountRecords(t, env, ownerAccount, authorityAccount, index, accountType)

	if accountType != commonpb.AccountType_LEGACY_PRIMARY_2022 {
		require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountRecords.General))
	}

	if accountType == commonpb.AccountType_TEMPORARY_INCOMING {
		// Need an initial action to allow rotation checks to pass
		actionRecord := &action.Record{
			IntentType: intent.OpenAccounts,
			Intent:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			ActionType: action.OpenAccount,
			Source:     accountRecords.General.TokenAccount,
			State:      action.StatePending,
		}
		require.NoError(t, env.data.PutAllActions(env.ctx, actionRecord))
	}

	if accountRecords.IsTimelock() {
		accountRecords.Timelock.VaultState = timelock_token_v1.StateLocked
		accountRecords.Timelock.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))
	}

	return accountRecords
}

func getDefaultTestAccountRecords(t *testing.T, env testEnv, ownerAccount, authorityAccount *common.Account, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	var dataVerstion timelock_token_v1.TimelockDataVersion
	if accountType == commonpb.AccountType_LEGACY_PRIMARY_2022 {
		dataVerstion = timelock_token_v1.DataVersionLegacy
	} else {
		dataVerstion = timelock_token_v1.DataVersion1
	}

	var tokenAccount *common.Account
	var mintAccount *common.Account
	var timelockRecord *timelock.Record
	var err error

	if accountType == commonpb.AccountType_SWAP {
		mintAccount = common.UsdcMintAccount

		tokenAccount, err = authorityAccount.ToAssociatedTokenAccount(mintAccount)
		require.NoError(t, err)
	} else {
		mintAccount = common.KinMintAccount

		timelockAccounts, err := authorityAccount.GetTimelockAccounts(dataVerstion, mintAccount)
		require.NoError(t, err)
		timelockRecord = timelockAccounts.ToDBRecord()

		tokenAccount = timelockAccounts.Vault
	}

	accountInfoRecord := &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: authorityAccount.PublicKey().ToBase58(),
		TokenAccount:     tokenAccount.PublicKey().ToBase58(),
		MintAccount:      mintAccount.PublicKey().ToBase58(),

		AccountType: accountType,

		Index: index,
	}

	if accountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP {
		accountInfoRecord.RelationshipTo = pointer.String(fmt.Sprintf("app%d.com", rand.Int()))
	}

	return &common.AccountRecords{
		General:  accountInfoRecord,
		Timelock: timelockRecord,
	}
}

func setupCachedBalance(t *testing.T, env testEnv, accountRecords *common.AccountRecords, balance uint64) {
	if accountRecords.Timelock.DataVersion == timelock_token_v1.DataVersionLegacy {
		paymentRecord := &payment.Record{
			Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Destination: accountRecords.General.TokenAccount,
			Quantity:    balance,

			Rendezvous: "",
			IsExternal: true,

			TransactionId: fmt.Sprintf("txn%d", rand.Uint64()),

			ConfirmationState: transaction.ConfirmationFinalized,

			ExchangeCurrency: string(currency.KIN),
			ExchangeRate:     1.0,
			UsdMarketValue:   1.0,

			BlockId: 12345,

			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.CreatePayment(env.ctx, paymentRecord))
	} else {
		depositRecord := &deposit.Record{
			Signature:      fmt.Sprintf("txn%d", rand.Uint64()),
			Destination:    accountRecords.General.TokenAccount,
			Amount:         balance,
			UsdMarketValue: 1,

			ConfirmationState: transaction.ConfirmationFinalized,
			Slot:              12345,
		}
		require.NoError(t, env.data.SaveExternalDeposit(env.ctx, depositRecord))
	}
}

func setupOpenAccountsIntent(t *testing.T, env testEnv, ownerAccount *common.Account) {
	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.OpenAccounts,

		InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),

		OpenAccountsMetadata: &intent.OpenAccountsMetadata{},

		State: intent.StatePending,
	}

	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
}

func setupPrivacyMigration2022Intent(t *testing.T, env testEnv, ownerAccount *common.Account) {
	tokenAccount, err := ownerAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
	require.NoError(t, err)

	balance, err := balance.CalculateFromCache(env.ctx, env.data, tokenAccount)
	require.NoError(t, err)

	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.MigrateToPrivacy2022,

		InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),

		MigrateToPrivacy2022Metadata: &intent.MigrateToPrivacy2022Metadata{
			Quantity: balance,
		},

		State: intent.StatePending,
	}

	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
}
