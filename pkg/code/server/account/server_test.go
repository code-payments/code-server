package account

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math/rand"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountpb "github.com/code-payments/code-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/currency"
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

func TestIsCodeAccount_NotManagedByCode(t *testing.T) {
	for i := 0; i < 4; i++ {
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
	swapDerivedOwner := testutil.NewRandomAccount(t)

	primaryAccountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_PRIMARY)
	bucketAccountRecords := setupAccountRecords(t, env, ownerAccount, bucketDerivedOwner, 0, commonpb.AccountType_BUCKET_100_KIN)
	setupAccountRecords(t, env, ownerAccount, swapDerivedOwner, 0, commonpb.AccountType_SWAP)
	setupAccountRecords(t, env, ownerAccount, tempIncomingDerivedOwner, 2, commonpb.AccountType_TEMPORARY_INCOMING)
	setupCachedBalance(t, env, bucketAccountRecords, common.ToCoreMintQuarks(100))
	setupCachedBalance(t, env, primaryAccountRecords, common.ToCoreMintQuarks(42))

	otherOwnerAccount := testutil.NewRandomAccount(t)
	setupAccountRecords(t, env, otherOwnerAccount, otherOwnerAccount, 0, commonpb.AccountType_PRIMARY)
	setupAccountRecords(t, env, otherOwnerAccount, testutil.NewRandomAccount(t), 0, commonpb.AccountType_BUCKET_100_KIN)
	setupAccountRecords(t, env, otherOwnerAccount, testutil.NewRandomAccount(t), 2, commonpb.AccountType_TEMPORARY_INCOMING)

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 4)

	for _, authority := range []*common.Account{
		ownerAccount,
		bucketDerivedOwner,
		tempIncomingDerivedOwner,
		swapDerivedOwner,
	} {
		var tokenAccount *common.Account
		if authority.PublicKey().ToBase58() == swapDerivedOwner.PublicKey().ToBase58() {
			tokenAccount, err = authority.ToAssociatedTokenAccount(common.UsdcMintAccount)
			require.NoError(t, err)
		} else {
			timelockAccounts, err := authority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
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
			assert.EqualValues(t, common.ToCoreMintQuarks(42), accountInfo.Balance)
		case bucketDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_BUCKET_100_KIN, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, common.ToCoreMintQuarks(100), accountInfo.Balance)
		case tempIncomingDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_TEMPORARY_INCOMING, accountInfo.AccountType)
			assert.EqualValues(t, 2, accountInfo.Index)
			assert.EqualValues(t, 0, accountInfo.Balance)
		case swapDerivedOwner.PublicKey().ToBase58():
			assert.Equal(t, commonpb.AccountType_SWAP, accountInfo.AccountType)
			assert.EqualValues(t, 0, accountInfo.Index)
			assert.EqualValues(t, 0, accountInfo.Balance)
		default:
			require.Fail(t, "unexpected authority")
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
			assert.Equal(t, common.CoreMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
		}

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
	for _, tc := range []struct {
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
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateUnlocked,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: true,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
		{
			balance:                  42,
			timelockState:            timelock_token_v1.StateClosed,
			simulateClaimInCode:      false,
			simulateAutoReturnInCode: true,
			expectedBalanceSource:    accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE,
			expectedBlockchainState:  accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_DOES_NOT_EXIST,
			expectedManagementState:  accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
			expectedClaimState:       accountpb.TokenAccountInfo_CLAIM_STATE_CLAIMED,
		},
	} {
		ownerAccount := testutil.NewRandomAccount(t)
		timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
		require.NoError(t, err)

		req := &accountpb.GetTokenAccountInfosRequest{
			Owner: ownerAccount.ToProto(),
		}
		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		accountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, 0, commonpb.AccountType_REMOTE_SEND_GIFT_CARD)

		giftCardIssuedIntentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPublicPayment,

			InitiatorOwnerAccount: testutil.NewRandomAccount(t).PrivateKey().ToBase58(),

			SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
				DestinationTokenAccount: accountRecords.General.TokenAccount,
				Quantity:                common.ToCoreMintQuarks(10),

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
			IntentType: intent.SendPublicPayment,

			ActionId:   0,
			ActionType: action.NoPrivacyWithdraw,

			Source:      accountRecords.General.TokenAccount,
			Destination: pointer.String("primary"),
			Quantity:    nil,

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
		assert.Equal(t, common.CoreMintAccount.PublicKey().ToBytes(), accountInfo.Mint.Value)
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
		assert.EqualValues(t, giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency, accountInfo.OriginalExchangeData.Currency)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeRate, accountInfo.OriginalExchangeData.ExchangeRate)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPublicPaymentMetadata.NativeAmount, accountInfo.OriginalExchangeData.NativeAmount)
		assert.Equal(t, giftCardIssuedIntentRecord.SendPublicPaymentMetadata.Quantity, accountInfo.OriginalExchangeData.Quarks)

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
		timelockState timelock_token_v1.TimelockState
		block         uint64
		expected      accountpb.TokenAccountInfo_ManagementState
	}{
		{
			timelockState: timelock_token_v1.StateUnknown,
			block:         0,
			expected:      accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
		},
		{timelockState: timelock_token_v1.StateUnknown,
			block:    1,
			expected: accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNKNOWN,
		},
		{timelockState: timelock_token_v1.StateUnlocked,
			block:    2,
			expected: accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKED,
		},
		{
			timelockState: timelock_token_v1.StateWaitingForTimeout,
			block:         3,
			expected:      accountpb.TokenAccountInfo_MANAGEMENT_STATE_UNLOCKING,
		},
		{
			timelockState: timelock_token_v1.StateLocked,
			block:         4,
			expected:      accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED,
		},
		{
			timelockState: timelock_token_v1.StateClosed,
			block:         5,
			expected:      accountpb.TokenAccountInfo_MANAGEMENT_STATE_CLOSED,
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

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	// swapAuthorityAccount := testutil.NewRandomAccount(t)
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
}

func setupAccountRecords(t *testing.T, env testEnv, ownerAccount, authorityAccount *common.Account, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	accountRecords := getDefaultTestAccountRecords(t, env, ownerAccount, authorityAccount, index, accountType)

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountRecords.General))

	if accountRecords.IsTimelock() {
		accountRecords.Timelock.VaultState = timelock_token_v1.StateLocked
		accountRecords.Timelock.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))
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

	return accountRecords
}

func getDefaultTestAccountRecords(t *testing.T, env testEnv, ownerAccount, authorityAccount *common.Account, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	var tokenAccount *common.Account
	var mintAccount *common.Account
	var timelockRecord *timelock.Record
	var err error

	if accountType == commonpb.AccountType_SWAP {
		mintAccount = common.UsdcMintAccount

		tokenAccount, err = authorityAccount.ToAssociatedTokenAccount(mintAccount)
		require.NoError(t, err)
	} else {
		mintAccount = common.CoreMintAccount

		timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, mintAccount)
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

	return &common.AccountRecords{
		General:  accountInfoRecord,
		Timelock: timelockRecord,
	}
}

func setupCachedBalance(t *testing.T, env testEnv, accountRecords *common.AccountRecords, balance uint64) {
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
