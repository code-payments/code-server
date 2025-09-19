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
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
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

	coreVmConfig := testutil.NewRandomVmConfig(t, true)
	swapVmConfig := testutil.NewRandomVmConfig(t, false)

	ownerAccount := testutil.NewRandomAccount(t)
	swapAuthorityAccount := testutil.NewRandomAccount(t)

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
	setupAccountRecords(t, env, ownerAccount, ownerAccount, coreVmConfig, 0, commonpb.AccountType_PRIMARY)
	setupAccountRecords(t, env, ownerAccount, swapAuthorityAccount, swapVmConfig, 0, commonpb.AccountType_SWAP)

	resp, err = env.client.IsCodeAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)
}

func TestIsCodeAccount_NotManagedByCode(t *testing.T) {
	for _, unmanagedState := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateUnlocked,
	} {
		env, cleanup := setup(t)
		defer cleanup()

		coreVmConfig := testutil.NewRandomVmConfig(t, true)

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
		allAccountRecords = append(allAccountRecords, setupAccountRecords(t, env, ownerAccount, ownerAccount, coreVmConfig, 0, commonpb.AccountType_PRIMARY))

		resp, err = env.client.IsCodeAccount(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.IsCodeAccountResponse_OK, resp.Result)

		allAccountRecords[0].Timelock.VaultState = unmanagedState
		allAccountRecords[0].Timelock.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, allAccountRecords[0].Timelock))

		resp, err = env.client.IsCodeAccount(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, accountpb.IsCodeAccountResponse_UNLOCKED_TIMELOCK_ACCOUNT, resp.Result)
	}
}

func TestGetTokenAccountInfos_UserAccounts_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	coreVmConfig := testutil.NewRandomVmConfig(t, true)
	jeffyVmConfig := testutil.NewRandomVmConfig(t, false)
	swapVmConfig := testutil.NewRandomVmConfig(t, false)

	ownerAccount := testutil.NewRandomAccount(t)

	req := &accountpb.GetTokenAccountInfosRequest{
		Owner: ownerAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	poolAuthority1 := testutil.NewRandomAccount(t)
	poolAuthority2 := testutil.NewRandomAccount(t)
	swapAuthority := testutil.NewRandomAccount(t)

	primaryCoreMintAccountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, coreVmConfig, 0, commonpb.AccountType_PRIMARY)
	pool1CoreMintAccountRecords := setupAccountRecords(t, env, ownerAccount, poolAuthority1, coreVmConfig, 0, commonpb.AccountType_POOL)
	pool2CoreMintAccountRecords := setupAccountRecords(t, env, ownerAccount, poolAuthority2, coreVmConfig, 1, commonpb.AccountType_POOL)
	primaryJeffyMintAccountRecords := setupAccountRecords(t, env, ownerAccount, ownerAccount, jeffyVmConfig, 0, commonpb.AccountType_PRIMARY)
	setupAccountRecords(t, env, ownerAccount, swapAuthority, swapVmConfig, 0, commonpb.AccountType_SWAP)

	setupCachedBalance(t, env, primaryCoreMintAccountRecords, common.ToCoreMintQuarks(42))
	setupCachedBalance(t, env, pool1CoreMintAccountRecords, common.ToCoreMintQuarks(88))
	setupCachedBalance(t, env, pool2CoreMintAccountRecords, common.ToCoreMintQuarks(123))
	setupCachedBalance(t, env, primaryJeffyMintAccountRecords, currencycreator.ToQuarks(98765))

	resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
	assert.Len(t, resp.TokenAccountInfos, 5)
	assert.EqualValues(t, 2, resp.NextPoolIndex)

	for _, tc := range []struct {
		authority *common.Account
		vmConfigs []*common.VmConfig
	}{
		{ownerAccount, []*common.VmConfig{coreVmConfig, jeffyVmConfig}},
		{poolAuthority1, []*common.VmConfig{coreVmConfig}},
		{poolAuthority2, []*common.VmConfig{coreVmConfig}},
		{swapAuthority, []*common.VmConfig{swapVmConfig}},
	} {
		for _, vmConfig := range tc.vmConfigs {
			var tokenAccount *common.Account
			if tc.authority.PublicKey().ToBase58() == swapAuthority.PublicKey().ToBase58() {
				tokenAccount, err = tc.authority.ToAssociatedTokenAccount(vmConfig.Mint)
				require.NoError(t, err)
			} else {
				timelockAccounts, err := tc.authority.GetTimelockAccounts(vmConfig)
				require.NoError(t, err)
				tokenAccount = timelockAccounts.Vault
			}

			accountInfo, ok := resp.TokenAccountInfos[tokenAccount.PublicKey().ToBase58()]
			require.True(t, ok)

			assert.Equal(t, tokenAccount.PublicKey().ToBytes(), accountInfo.Address.Value)
			assert.Equal(t, ownerAccount.PublicKey().ToBytes(), accountInfo.Owner.Value)
			assert.Equal(t, tc.authority.PublicKey().ToBytes(), accountInfo.Authority.Value)
			assert.Equal(t, vmConfig.Mint.PublicKey().ToBytes(), accountInfo.Mint.Value)

			switch tc.authority.PublicKey().ToBase58() {
			case ownerAccount.PublicKey().ToBase58():
				assert.Equal(t, commonpb.AccountType_PRIMARY, accountInfo.AccountType)
				assert.EqualValues(t, 0, accountInfo.Index)
				switch vmConfig.Mint.PublicKey().ToBase58() {
				case common.CoreMintAccount.PublicKey().ToBase58():
					assert.EqualValues(t, common.ToCoreMintQuarks(42), accountInfo.Balance)
				case jeffyVmConfig.Mint.PublicKey().ToBase58():
					assert.EqualValues(t, currencycreator.ToQuarks(98765), accountInfo.Balance)
				default:
					require.Fail(t, "unexpected mint")
				}
			case swapAuthority.PublicKey().ToBase58():
				assert.Equal(t, commonpb.AccountType_SWAP, accountInfo.AccountType)
				assert.EqualValues(t, 0, accountInfo.Index)
				assert.EqualValues(t, 0, accountInfo.Balance)
			case poolAuthority1.PublicKey().ToBase58():
				assert.Equal(t, commonpb.AccountType_POOL, accountInfo.AccountType)
				assert.EqualValues(t, 0, accountInfo.Index)
				assert.EqualValues(t, common.ToCoreMintQuarks(88), accountInfo.Balance)
			case poolAuthority2.PublicKey().ToBase58():
				assert.Equal(t, commonpb.AccountType_POOL, accountInfo.AccountType)
				assert.EqualValues(t, 1, accountInfo.Index)
				assert.EqualValues(t, common.ToCoreMintQuarks(123), accountInfo.Balance)
			default:
				require.Fail(t, "unexpected authority")
			}

			if accountInfo.AccountType == commonpb.AccountType_SWAP {
				assert.Equal(t, accountpb.TokenAccountInfo_BALANCE_SOURCE_BLOCKCHAIN, accountInfo.BalanceSource)
				assert.Equal(t, accountpb.TokenAccountInfo_MANAGEMENT_STATE_NONE, accountInfo.ManagementState)
				assert.Equal(t, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_UNKNOWN, accountInfo.BlockchainState)
			} else {
				assert.Equal(t, accountpb.TokenAccountInfo_BALANCE_SOURCE_CACHE, accountInfo.BalanceSource)
				assert.Equal(t, accountpb.TokenAccountInfo_MANAGEMENT_STATE_LOCKED, accountInfo.ManagementState)
				assert.Equal(t, accountpb.TokenAccountInfo_BLOCKCHAIN_STATE_EXISTS, accountInfo.BlockchainState)
			}

			assert.Equal(t, accountpb.TokenAccountInfo_CLAIM_STATE_UNKNOWN, accountInfo.ClaimState)
			assert.Nil(t, accountInfo.OriginalExchangeData)
		}
	}

	primaryAccountInfoRecordsByMint, err := env.data.GetLatestAccountInfoByOwnerAddressAndType(env.ctx, ownerAccount.PublicKey().ToBase58(), commonpb.AccountType_PRIMARY)
	require.NoError(t, err)
	assert.True(t, primaryAccountInfoRecordsByMint[common.CoreMintAccount.PublicKey().ToBase58()].RequiresDepositSync)
	assert.False(t, primaryAccountInfoRecordsByMint[jeffyVmConfig.Mint.PublicKey().ToBase58()].RequiresDepositSync)
}

func TestGetTokenAccountInfos_RemoteSendGiftCard_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	coreVmConfig := testutil.NewRandomVmConfig(t, true)

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
		giftCardIssuerOwnerAccount := testutil.NewRandomAccount(t)
		giftCardOwnerAccount := testutil.NewRandomAccount(t)

		accountRecords := setupAccountRecords(t, env, giftCardOwnerAccount, giftCardOwnerAccount, coreVmConfig, 0, commonpb.AccountType_REMOTE_SEND_GIFT_CARD)

		giftCardIssuedIntentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPublicPayment,

			MintAccount: common.CoreMintAccount.PublicKey().ToBase58(),

			InitiatorOwnerAccount: giftCardIssuerOwnerAccount.PublicKey().ToBase58(),

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

		for _, requestingOwnerAccount := range []*common.Account{
			nil,
			testutil.NewRandomAccount(t),
			giftCardIssuerOwnerAccount,
		} {
			timelockAccounts, err := giftCardOwnerAccount.GetTimelockAccounts(coreVmConfig)
			require.NoError(t, err)

			req := &accountpb.GetTokenAccountInfosRequest{
				Owner: giftCardOwnerAccount.ToProto(),
			}
			if requestingOwnerAccount != nil {
				req.RequestingOwner = requestingOwnerAccount.ToProto()
			}
			reqBytes, err := proto.Marshal(req)
			require.NoError(t, err)
			sig1 := &commonpb.Signature{
				Value: ed25519.Sign(giftCardOwnerAccount.PrivateKey().ToBytes(), reqBytes),
			}
			if requestingOwnerAccount != nil {
				sig2 := &commonpb.Signature{
					Value: ed25519.Sign(requestingOwnerAccount.PrivateKey().ToBytes(), reqBytes),
				}
				req.RequestingOwnerSignature = sig2
			}
			req.Signature = sig1

			resp, err := env.client.GetTokenAccountInfos(env.ctx, req)
			require.NoError(t, err)
			assert.Equal(t, accountpb.GetTokenAccountInfosResponse_OK, resp.Result)
			assert.Len(t, resp.TokenAccountInfos, 1)
			assert.EqualValues(t, 0, resp.NextPoolIndex)

			accountInfo, ok := resp.TokenAccountInfos[timelockAccounts.Vault.PublicKey().ToBase58()]
			require.True(t, ok)

			assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, accountInfo.AccountType)
			assert.Equal(t, giftCardOwnerAccount.PublicKey().ToBytes(), accountInfo.Owner.Value)
			assert.Equal(t, giftCardOwnerAccount.PublicKey().ToBytes(), accountInfo.Authority.Value)
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

			assert.Equal(t, requestingOwnerAccount != nil && requestingOwnerAccount == giftCardIssuerOwnerAccount, accountInfo.IsGiftCardIssuer)

			accountInfoRecordsByMint, err := env.data.GetLatestAccountInfoByOwnerAddressAndType(env.ctx, giftCardOwnerAccount.PublicKey().ToBase58(), commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
			require.NoError(t, err)
			assert.False(t, accountInfoRecordsByMint[common.CoreMintAccount.PublicKey().ToBase58()].RequiresDepositSync)
		}
	}
}

func TestGetTokenAccountInfos_BlockchainState(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	coreVmConfig := testutil.NewRandomVmConfig(t, true)

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

		accountRecords := getDefaultTestAccountRecords(t, ownerAccount, ownerAccount, coreVmConfig, 0, commonpb.AccountType_PRIMARY)
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

	coreVmConfig := testutil.NewRandomVmConfig(t, true)

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

		accountRecords := getDefaultTestAccountRecords(t, ownerAccount, ownerAccount, coreVmConfig, 0, commonpb.AccountType_PRIMARY)
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
	requestingOwnerAccount := testutil.NewRandomAccount(t)
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

	getTokenAccountInfosReq = &accountpb.GetTokenAccountInfosRequest{
		Owner:           ownerAccount.ToProto(),
		RequestingOwner: requestingOwnerAccount.ToProto(),
	}
	reqBytes, err = proto.Marshal(getTokenAccountInfosReq)
	require.NoError(t, err)
	getTokenAccountInfosReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}

	_, err = env.client.GetTokenAccountInfos(env.ctx, getTokenAccountInfosReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	getTokenAccountInfosReq = &accountpb.GetTokenAccountInfosRequest{
		Owner:           ownerAccount.ToProto(),
		RequestingOwner: requestingOwnerAccount.ToProto(),
	}
	reqBytes, err = proto.Marshal(getTokenAccountInfosReq)
	require.NoError(t, err)
	sig1 := &commonpb.Signature{
		Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
	}
	sig2 := &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}
	getTokenAccountInfosReq.Signature = sig1
	getTokenAccountInfosReq.RequestingOwnerSignature = sig2

	_, err = env.client.GetTokenAccountInfos(env.ctx, getTokenAccountInfosReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func setupAccountRecords(t *testing.T, env testEnv, ownerAccount, authorityAccount *common.Account, vmConfig *common.VmConfig, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	accountRecords := getDefaultTestAccountRecords(t, ownerAccount, authorityAccount, vmConfig, index, accountType)

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountRecords.General))

	if accountRecords.IsTimelock() {
		accountRecords.Timelock.VaultState = timelock_token_v1.StateLocked
		accountRecords.Timelock.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, accountRecords.Timelock))
	}

	return accountRecords
}

func getDefaultTestAccountRecords(t *testing.T, ownerAccount, authorityAccount *common.Account, vmConfig *common.VmConfig, index uint64, accountType commonpb.AccountType) *common.AccountRecords {
	var tokenAccount *common.Account
	var timelockRecord *timelock.Record
	var err error

	if accountType == commonpb.AccountType_SWAP {
		tokenAccount, err = authorityAccount.ToAssociatedTokenAccount(vmConfig.Mint)
		require.NoError(t, err)
	} else {
		timelockAccounts, err := authorityAccount.GetTimelockAccounts(vmConfig)
		require.NoError(t, err)
		timelockRecord = timelockAccounts.ToDBRecord()

		tokenAccount = timelockAccounts.Vault
	}

	accountInfoRecord := &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: authorityAccount.PublicKey().ToBase58(),
		TokenAccount:     tokenAccount.PublicKey().ToBase58(),
		MintAccount:      vmConfig.Mint.PublicKey().ToBase58(),

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
