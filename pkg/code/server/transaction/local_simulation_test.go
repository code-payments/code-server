package transaction_v2

/*
import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestLocalSimulation_HappyPath(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	newAccount := testutil.NewRandomAccount(t)
	existingAccount1 := testutil.NewRandomAccount(t)
	existingAccount2 := testutil.NewRandomAccount(t)
	existingAccount3 := testutil.NewRandomAccount(t)
	externalATA := testutil.NewRandomAccount(t)

	env.setupTimelockRecord(t, existingAccount1, timelock_token_v1.StateLocked)
	env.setupTimelockRecord(t, existingAccount2, timelock_token_v1.StateLocked)
	env.setupTimelockRecord(t, existingAccount3, timelock_token_v1.StateLocked)

	env.setupCachedBalance(t, existingAccount1, 102)
	env.setupCachedBalance(t, existingAccount2, 20)
	env.setupCachedBalance(t, existingAccount3, 0)

	actions := []*transactionpb.Action{
		getOpenAccountActionForLocalSimulation(t, newAccount),

		getNoPrivacyTransferActionForLocalSimulation(t, existingAccount1, getTimelockVault(t, newAccount), 1),
		getNoPrivacyWithdrawActionForLocalSimulation(t, existingAccount1, getTimelockVault(t, newAccount), 100),

		getTemporaryPrivacyTransferActionForLocalSimulation(t, newAccount, getTimelockVault(t, newAccount), 101),
		getTemporaryPrivacyExchangeActionForLocalSimulation(t, newAccount, getTimelockVault(t, newAccount), 50),

		getTemporaryPrivacyTransferActionForLocalSimulation(t, existingAccount2, getTimelockVault(t, newAccount), 3),
		getTemporaryPrivacyTransferActionForLocalSimulation(t, newAccount, getTimelockVault(t, existingAccount2), 4),
		getTemporaryPrivacyTransferActionForLocalSimulation(t, existingAccount2, externalATA, 8),

		getTemporaryPrivacyTransferActionForLocalSimulation(t, newAccount, getTimelockVault(t, existingAccount3), 1),
		getTemporaryPrivacyExchangeActionForLocalSimulation(t, newAccount, getTimelockVault(t, existingAccount3), 10),
		getNoPrivacyTransferActionForLocalSimulation(t, existingAccount3, getTimelockVault(t, newAccount), 11),
		getCloseEmptyAccountActionForLocalSimulation(t, existingAccount3),

		getFeePaymentActionForLocalSimulation(t, newAccount, 1),
	}

	simResult, err := env.LocalSimulation(t, actions)
	require.NoError(t, err)
	require.Len(t, simResult.SimulationsByAccount, 5)

	accountSimulation, ok := simResult.SimulationsByAccount[getTimelockVault(t, newAccount).PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, getTimelockVault(t, newAccount).PublicKey().ToBase58(), accountSimulation.TokenAccount.PublicKey().ToBase58())

	assert.True(t, accountSimulation.Opened)
	require.NotNil(t, accountSimulation.OpenAction)
	assert.EqualValues(t, 0, accountSimulation.OpenAction.Id)

	assert.False(t, accountSimulation.Closed)
	assert.Nil(t, accountSimulation.CloseAction)

	require.Len(t, accountSimulation.Transfers, 12)
	assert.EqualValues(t, 99, accountSimulation.GetDeltaQuarks())

	assert.EqualValues(t, 1, accountSimulation.Transfers[0].Action.Id)
	assert.EqualValues(t, 1, accountSimulation.Transfers[0].DeltaQuarks)

	assert.EqualValues(t, 2, accountSimulation.Transfers[1].Action.Id)
	assert.EqualValues(t, 100, accountSimulation.Transfers[1].DeltaQuarks)

	assert.EqualValues(t, 3, accountSimulation.Transfers[2].Action.Id)
	assert.EqualValues(t, -101, accountSimulation.Transfers[2].DeltaQuarks)

	assert.EqualValues(t, 3, accountSimulation.Transfers[3].Action.Id)
	assert.EqualValues(t, 101, accountSimulation.Transfers[3].DeltaQuarks)

	assert.EqualValues(t, 4, accountSimulation.Transfers[4].Action.Id)
	assert.EqualValues(t, -50, accountSimulation.Transfers[4].DeltaQuarks)

	assert.EqualValues(t, 4, accountSimulation.Transfers[5].Action.Id)
	assert.EqualValues(t, 50, accountSimulation.Transfers[5].DeltaQuarks)

	assert.EqualValues(t, 5, accountSimulation.Transfers[6].Action.Id)
	assert.EqualValues(t, 3, accountSimulation.Transfers[6].DeltaQuarks)

	assert.EqualValues(t, 6, accountSimulation.Transfers[7].Action.Id)
	assert.EqualValues(t, -4, accountSimulation.Transfers[7].DeltaQuarks)

	assert.EqualValues(t, 8, accountSimulation.Transfers[8].Action.Id)
	assert.EqualValues(t, -1, accountSimulation.Transfers[8].DeltaQuarks)

	assert.EqualValues(t, 9, accountSimulation.Transfers[9].Action.Id)
	assert.EqualValues(t, -10, accountSimulation.Transfers[9].DeltaQuarks)

	assert.EqualValues(t, 10, accountSimulation.Transfers[10].Action.Id)
	assert.EqualValues(t, 11, accountSimulation.Transfers[10].DeltaQuarks)

	assert.EqualValues(t, 12, accountSimulation.Transfers[11].Action.Id)
	assert.EqualValues(t, -1, accountSimulation.Transfers[11].DeltaQuarks)

	assertCorrectTransferSimulationFlags(t, accountSimulation.Transfers)

	accountSimulation, ok = simResult.SimulationsByAccount[getTimelockVault(t, existingAccount1).PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, getTimelockVault(t, existingAccount1).PublicKey().ToBase58(), accountSimulation.TokenAccount.PublicKey().ToBase58())

	assert.False(t, accountSimulation.Opened)
	assert.Nil(t, accountSimulation.OpenAction)

	assert.True(t, accountSimulation.Closed)
	require.NotNil(t, accountSimulation.CloseAction)
	assert.EqualValues(t, 2, accountSimulation.CloseAction.Id)

	require.Len(t, accountSimulation.Transfers, 2)
	assert.EqualValues(t, -101, accountSimulation.GetDeltaQuarks())

	assert.EqualValues(t, 1, accountSimulation.Transfers[0].Action.Id)
	assert.EqualValues(t, -1, accountSimulation.Transfers[0].DeltaQuarks)

	assert.EqualValues(t, 2, accountSimulation.Transfers[1].Action.Id)
	assert.EqualValues(t, -100, accountSimulation.Transfers[1].DeltaQuarks)

	assertCorrectTransferSimulationFlags(t, accountSimulation.Transfers)

	accountSimulation, ok = simResult.SimulationsByAccount[getTimelockVault(t, existingAccount2).PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, getTimelockVault(t, existingAccount2).PublicKey().ToBase58(), accountSimulation.TokenAccount.PublicKey().ToBase58())

	assert.False(t, accountSimulation.Opened)
	assert.Nil(t, accountSimulation.OpenAction)

	assert.False(t, accountSimulation.Closed)
	assert.Nil(t, accountSimulation.CloseAction)

	require.Len(t, accountSimulation.Transfers, 3)
	assert.EqualValues(t, -7, accountSimulation.GetDeltaQuarks())

	assert.EqualValues(t, 5, accountSimulation.Transfers[0].Action.Id)
	assert.EqualValues(t, -3, accountSimulation.Transfers[0].DeltaQuarks)

	assert.EqualValues(t, 6, accountSimulation.Transfers[1].Action.Id)
	assert.EqualValues(t, 4, accountSimulation.Transfers[1].DeltaQuarks)

	assert.EqualValues(t, 7, accountSimulation.Transfers[2].Action.Id)
	assert.EqualValues(t, -8, accountSimulation.Transfers[2].DeltaQuarks)

	assertCorrectTransferSimulationFlags(t, accountSimulation.Transfers)

	accountSimulation, ok = simResult.SimulationsByAccount[getTimelockVault(t, existingAccount3).PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, getTimelockVault(t, existingAccount3).PublicKey().ToBase58(), accountSimulation.TokenAccount.PublicKey().ToBase58())

	assert.False(t, accountSimulation.Opened)
	assert.Nil(t, accountSimulation.OpenAction)

	assert.True(t, accountSimulation.Closed)
	require.NotNil(t, accountSimulation.CloseAction)
	assert.EqualValues(t, 11, accountSimulation.CloseAction.Id)

	require.Len(t, accountSimulation.Transfers, 3)
	assert.EqualValues(t, 0, accountSimulation.GetDeltaQuarks())

	assert.EqualValues(t, 8, accountSimulation.Transfers[0].Action.Id)
	assert.EqualValues(t, 1, accountSimulation.Transfers[0].DeltaQuarks)

	assert.EqualValues(t, 9, accountSimulation.Transfers[1].Action.Id)
	assert.EqualValues(t, 10, accountSimulation.Transfers[1].DeltaQuarks)

	assert.EqualValues(t, 10, accountSimulation.Transfers[2].Action.Id)
	assert.EqualValues(t, -11, accountSimulation.Transfers[2].DeltaQuarks)

	assertCorrectTransferSimulationFlags(t, accountSimulation.Transfers)

	accountSimulation, ok = simResult.SimulationsByAccount[externalATA.PublicKey().ToBase58()]
	require.True(t, ok)

	assert.Equal(t, externalATA.PublicKey().ToBase58(), accountSimulation.TokenAccount.PublicKey().ToBase58())

	assert.False(t, accountSimulation.Opened)
	assert.Nil(t, accountSimulation.OpenAction)

	assert.False(t, accountSimulation.Closed)
	assert.Nil(t, accountSimulation.CloseAction)

	require.Len(t, accountSimulation.Transfers, 1)
	assert.EqualValues(t, 8, accountSimulation.GetDeltaQuarks())

	assert.EqualValues(t, 7, accountSimulation.Transfers[0].Action.Id)
	assert.EqualValues(t, 8, accountSimulation.Transfers[0].DeltaQuarks)

	assertCorrectTransferSimulationFlags(t, accountSimulation.Transfers)

	openedAccounts := simResult.GetOpenedAccounts()
	require.Len(t, openedAccounts, 1)
	assert.Equal(t, getTimelockVault(t, newAccount).PublicKey().ToBase58(), openedAccounts[0].TokenAccount.PublicKey().ToBase58())

	closedAccounts := simResult.GetClosedAccounts()
	require.Len(t, closedAccounts, 2)
	closedAccountsByVault := make(map[string]struct{})
	closedAccountsByVault[closedAccounts[0].TokenAccount.PublicKey().ToBase58()] = struct{}{}
	closedAccountsByVault[closedAccounts[1].TokenAccount.PublicKey().ToBase58()] = struct{}{}
	_, ok = closedAccountsByVault[getTimelockVault(t, existingAccount1).PublicKey().ToBase58()]
	assert.True(t, ok)
	_, ok = closedAccountsByVault[getTimelockVault(t, existingAccount3).PublicKey().ToBase58()]
	assert.True(t, ok)

	assert.True(t, simResult.HasAnyFeePayments())
	assert.Equal(t, 1, simResult.CountFeePayments())
	feePayments := simResult.GetFeePayments()
	require.Len(t, feePayments, 1)
	assert.EqualValues(t, 12, feePayments[0].Action.Id)
}

func TestLocalSimulation_NoActions(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)
	simResult, err := env.LocalSimulation(t, nil)
	require.NoError(t, err)
	assert.Empty(t, simResult.SimulationsByAccount)
}

func TestLocalSimulation_InsufficientBalanceForTransfer(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	env.setupTimelockRecord(t, authority, timelock_token_v1.StateLocked)
	env.setupCachedBalance(t, authority, 2)

	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 3),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 3),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 3),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 3),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getNoPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: insufficient balance to perform action", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_CloseAccountWithBalance(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	env.setupTimelockRecord(t, authority, timelock_token_v1.StateLocked)
	env.setupCachedBalance(t, authority, 2)

	funder := testutil.NewRandomAccount(t)
	env.setupTimelockRecord(t, funder, timelock_token_v1.StateLocked)
	env.setupCachedBalance(t, funder, 2)

	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 2),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, funder, getTimelockVault(t, authority), 1),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: attempt to close an account with a non-zero balance", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_OpenAnAlreadyOpenedAccount(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	actions := []*transactionpb.Action{
		getOpenAccountActionForLocalSimulation(t, authority),
		getOpenAccountActionForLocalSimulation(t, authority),
	}

	_, err := env.LocalSimulation(t, actions)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "actions[1]: account is already opened"))
}

func TestLocalSimulation_CloseAnAlreadyClosedAccount(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getOpenAccountActionForLocalSimulation(t, authority),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: account is already closed", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_ReopenAnAccount(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getOpenAccountActionForLocalSimulation(t, authority),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getOpenAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getOpenAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getOpenAccountActionForLocalSimulation(t, authority),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: account cannot be reopened", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_OpenAccountAfterTransferringToIt(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	existingAuthority := testutil.NewRandomAccount(t)
	env.setupTimelockRecord(t, existingAuthority, timelock_token_v1.StateLocked)
	env.setupCachedBalance(t, existingAuthority, 2)

	newAuthority := testutil.NewRandomAccount(t)
	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, existingAuthority, getTimelockVault(t, newAuthority), 2),
				getOpenAccountActionForLocalSimulation(t, newAuthority),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyTransferActionForLocalSimulation(t, existingAuthority, getTimelockVault(t, newAuthority), 1),
				getNoPrivacyTransferActionForLocalSimulation(t, existingAuthority, getTimelockVault(t, newAuthority), 1),
				getOpenAccountActionForLocalSimulation(t, newAuthority),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, existingAuthority, getTimelockVault(t, newAuthority), 1),
				getOpenAccountActionForLocalSimulation(t, newAuthority),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: opened an account after transferring funds to it", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_TransferFromClosedAccount(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getOpenAccountActionForLocalSimulation(t, authority),
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("actions[%d]: account is closed and cannot send/receive kin", len(tc.actions)-1)))
	}
}

func TestLocalSimulation_FeeStructureBecomesInvalidOverTime(t *testing.T) {
	microPaymentAmount := uint64(100)
	thirdPartyFeeAmount := uint64(20)

	// Simulate a test where the Code fee amount causes an insufficient balance
	// because the USD exchange rate fluctuated significantly against another
	// currency
	for _, codeFeeAmount := range []uint64{
		microPaymentAmount - 2*thirdPartyFeeAmount,
		microPaymentAmount - 2*thirdPartyFeeAmount + 1,
		microPaymentAmount - 2*thirdPartyFeeAmount - 1,
	} {
		env := setupLocalSimulationTestEnv(t)

		bucketAccount := testutil.NewRandomAccount(t)
		env.setupTimelockRecord(t, bucketAccount, timelock_token_v1.StateLocked)
		env.setupCachedBalance(t, bucketAccount, microPaymentAmount)

		tempOutgoingAccount := testutil.NewRandomAccount(t)
		env.setupTimelockRecord(t, tempOutgoingAccount, timelock_token_v1.StateLocked)

		externalATA := testutil.NewRandomAccount(t)

		actions := []*transactionpb.Action{
			getTemporaryPrivacyTransferActionForLocalSimulation(t, bucketAccount, getTimelockVault(t, tempOutgoingAccount), microPaymentAmount),
			getFeePaymentActionForLocalSimulation(t, tempOutgoingAccount, codeFeeAmount),
			getFeePaymentActionForLocalSimulation(t, tempOutgoingAccount, thirdPartyFeeAmount),
			getFeePaymentActionForLocalSimulation(t, tempOutgoingAccount, thirdPartyFeeAmount),
			getNoPrivacyWithdrawActionForLocalSimulation(t, tempOutgoingAccount, externalATA, 10),
		}

		_, err := env.LocalSimulation(t, actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "insufficient balance to perform action"))
	}
}

func TestLocalSimulation_NotManagedByCode(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	env.setupCachedBalance(t, authority, 2)

	for _, tc := range []struct {
		actions []*transactionpb.Action
	}{
		{
			[]*transactionpb.Action{
				getCloseEmptyAccountActionForLocalSimulation(t, authority),
			},
		},
		{
			[]*transactionpb.Action{
				getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
		{
			[]*transactionpb.Action{
				getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
			},
		},
	} {
		_, err := env.LocalSimulation(t, tc.actions)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "at least one account is no longer managed by code"))
	}
}

func TestLocalSimulation_InvalidTimelockVault(t *testing.T) {
	env := setupLocalSimulationTestEnv(t)

	authority := testutil.NewRandomAccount(t)
	invalidTimelockVault := testutil.NewRandomAccount(t)

	for _, action := range []*transactionpb.Action{
		getOpenAccountActionForLocalSimulation(t, authority),
		getCloseEmptyAccountActionForLocalSimulation(t, authority),
		getClosDormantAccountActionForLocalSimulation(t, authority),
		getNoPrivacyWithdrawActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
		getTemporaryPrivacyTransferActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
		getTemporaryPrivacyExchangeActionForLocalSimulation(t, authority, testutil.NewRandomAccount(t), 1),
	} {
		switch typed := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			typed.OpenAccount.Token = invalidTimelockVault.ToProto()
		case *transactionpb.Action_CloseEmptyAccount:
			typed.CloseEmptyAccount.Token = invalidTimelockVault.ToProto()
		case *transactionpb.Action_CloseDormantAccount:
			typed.CloseDormantAccount.Token = invalidTimelockVault.ToProto()
		case *transactionpb.Action_NoPrivacyWithdraw:
			typed.NoPrivacyWithdraw.Source = invalidTimelockVault.ToProto()
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			typed.TemporaryPrivacyTransfer.Source = invalidTimelockVault.ToProto()
		case *transactionpb.Action_TemporaryPrivacyExchange:
			typed.TemporaryPrivacyExchange.Source = invalidTimelockVault.ToProto()
		}

		_, err := env.LocalSimulation(t, []*transactionpb.Action{action})
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "token must be"))
	}

}

type localSimulationTestEnv struct {
	ctx  context.Context
	data code_data.Provider
}

func setupLocalSimulationTestEnv(t *testing.T) localSimulationTestEnv {
	data := code_data.NewTestDataProvider()
	testutil.SetupRandomSubsidizer(t, data)
	return localSimulationTestEnv{
		ctx:  context.Background(),
		data: data,
	}
}

func (env localSimulationTestEnv) setupTimelockRecord(t *testing.T, authority *common.Account, state timelock_token_v1.TimelockState) {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)
	timelockRecord := timelockAccounts.ToDBRecord()
	timelockRecord.VaultState = state
	timelockRecord.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))
}

func (env localSimulationTestEnv) setupCachedBalance(t *testing.T, authority *common.Account, amount uint64) {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	if amount > 0 {
		depositRecord := &deposit.Record{
			Signature:      fmt.Sprintf("txn%d", rand.Uint64()),
			Destination:    timelockAccounts.Vault.PublicKey().ToBase58(),
			Amount:         amount,
			UsdMarketValue: 1,

			ConfirmationState: transaction.ConfirmationFinalized,
			Slot:              12345,
		}
		require.NoError(t, env.data.SaveExternalDeposit(env.ctx, depositRecord))
	}
}

func (env localSimulationTestEnv) LocalSimulation(t *testing.T, actions []*transactionpb.Action) (*LocalSimulationResult, error) {
	for i, action := range actions {
		action.Id = uint32(i)
	}

	return LocalSimulation(env.ctx, env.data, actions)
}

func getOpenAccountActionForLocalSimulation(t *testing.T, authority *common.Account) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountAction{
				Authority: authority.ToProto(),
				Token:     timelockAccounts.Vault.ToProto(),
			},
		},
	}
}

func getCloseEmptyAccountActionForLocalSimulation(t *testing.T, authority *common.Account) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_CloseEmptyAccount{
			CloseEmptyAccount: &transactionpb.CloseEmptyAccountAction{
				Authority: authority.ToProto(),
				Token:     timelockAccounts.Vault.ToProto(),
			},
		},
	}
}

func getClosDormantAccountActionForLocalSimulation(t *testing.T, authority *common.Account) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountAction{
				Authority: authority.ToProto(),
				Token:     timelockAccounts.Vault.ToProto(),
			},
		},
	}
}

func getNoPrivacyWithdrawActionForLocalSimulation(t *testing.T, authority, destination *common.Account, amount uint64) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawAction{
				Authority:   authority.ToProto(),
				Source:      timelockAccounts.Vault.ToProto(),
				Destination: destination.ToProto(),
				Amount:      amount,
			},
		},
	}
}

func getNoPrivacyTransferActionForLocalSimulation(t *testing.T, authority, destination *common.Account, amount uint64) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferAction{
				Authority:   authority.ToProto(),
				Source:      timelockAccounts.Vault.ToProto(),
				Destination: destination.ToProto(),
				Amount:      amount,
			},
		},
	}
}

func getFeePaymentActionForLocalSimulation(t *testing.T, authority *common.Account, amount uint64) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_FeePayment{
			FeePayment: &transactionpb.FeePaymentAction{
				Authority: authority.ToProto(),
				Source:    timelockAccounts.Vault.ToProto(),
				Amount:    amount,
			},
		},
	}
}

func getTemporaryPrivacyTransferActionForLocalSimulation(t *testing.T, authority, destination *common.Account, amount uint64) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferAction{
				Authority:   authority.ToProto(),
				Source:      timelockAccounts.Vault.ToProto(),
				Destination: destination.ToProto(),
				Amount:      amount,
			},
		},
	}
}

func getTemporaryPrivacyExchangeActionForLocalSimulation(t *testing.T, authority, destination *common.Account, amount uint64) *transactionpb.Action {
	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	require.NoError(t, err)

	return &transactionpb.Action{
		Type: &transactionpb.Action_TemporaryPrivacyExchange{
			TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeAction{
				Authority:   authority.ToProto(),
				Source:      timelockAccounts.Vault.ToProto(),
				Destination: destination.ToProto(),
				Amount:      amount,
			},
		},
	}
}

func assertCorrectTransferSimulationFlags(t *testing.T, simulations []TransferSimulation) {
	for _, simulation := range simulations {
		switch simulation.Action.Type.(type) {
		case *transactionpb.Action_NoPrivacyTransfer:
			assert.False(t, simulation.IsPrivate)
			assert.False(t, simulation.IsWithdraw)
			assert.False(t, simulation.IsFee)
		case *transactionpb.Action_FeePayment:
			assert.False(t, simulation.IsPrivate)
			assert.False(t, simulation.IsWithdraw)
			assert.True(t, simulation.IsFee)
		case *transactionpb.Action_NoPrivacyWithdraw:
			assert.False(t, simulation.IsPrivate)
			assert.True(t, simulation.IsWithdraw)
			assert.False(t, simulation.IsFee)
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			assert.True(t, simulation.IsPrivate)
			assert.False(t, simulation.IsWithdraw)
			assert.False(t, simulation.IsFee)
		case *transactionpb.Action_TemporaryPrivacyExchange:
			assert.True(t, simulation.IsPrivate)
			assert.False(t, simulation.IsWithdraw)
			assert.False(t, simulation.IsFee)
		default:
			assert.Fail(t, "unhandled action type")
		}
	}
}
*/
