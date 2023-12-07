package balance

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/pointer"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestDefaultCalculationMethods_NewCodeAccount(t *testing.T) {
	env := setupBalanceTestEnv(t)

	newOwnerAccount := testutil.NewRandomAccount(t)
	newTokenAccount, err := newOwnerAccount.ToTimelockVault(getTimelockDataVersion(false))
	require.NoError(t, err)

	data := &balanceTestData{
		codeUsers: []*common.Account{newOwnerAccount},
	}

	setupBalanceTestData(t, env, data, balanceTestDataConf{})

	accountRecords, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, newOwnerAccount)
	require.NoError(t, err)

	balance, err := DefaultCalculation(env.ctx, env.data, newTokenAccount)
	require.NoError(t, err)
	assert.EqualValues(t, 0, balance)

	balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords[commonpb.AccountType_PRIMARY][0])
	require.NoError(t, err)
	require.Len(t, balanceByAccount, 1)
	assert.EqualValues(t, 0, balanceByAccount[newTokenAccount.PublicKey().ToBase58()])

	balanceByAccount, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, newTokenAccount)
	require.NoError(t, err)
	require.Len(t, balanceByAccount, 1)
	assert.EqualValues(t, 0, balanceByAccount[newTokenAccount.PublicKey().ToBase58()])
}

func TestDefaultCalculationMethods_DepositFromExternalWallet(t *testing.T) {
	for _, useLegacyDeposits := range []bool{true, false} {
		env := setupBalanceTestEnv(t)

		owner := testutil.NewRandomAccount(t)
		depositAccount, err := owner.ToTimelockVault(getTimelockDataVersion(useLegacyDeposits))
		require.NoError(t, err)

		externalAccount := testutil.NewRandomAccount(t)

		data := &balanceTestData{
			codeUsers: []*common.Account{owner},
			transactions: []balanceTestTransaction{
				// The following entries are added to the balance
				{source: externalAccount, destination: depositAccount, quantity: 1, transactionState: transaction.ConfirmationFinalized},
				{source: externalAccount, destination: depositAccount, quantity: 10, transactionState: transaction.ConfirmationFinalized},
				// The following entries aren't added to the balance because they aren't finalized
				{source: externalAccount, destination: depositAccount, quantity: 100, transactionState: transaction.ConfirmationFailed},
				{source: externalAccount, destination: depositAccount, quantity: 1000, transactionState: transaction.ConfirmationPending},
				{source: externalAccount, destination: depositAccount, quantity: 10000, transactionState: transaction.ConfirmationUnknown},
			},
		}
		setupBalanceTestData(t, env, data, balanceTestDataConf{
			useLegacyIntents:  useLegacyDeposits,
			useLegacyDeposits: useLegacyDeposits,
		})

		balance, err := DefaultCalculation(env.ctx, env.data, depositAccount)
		require.NoError(t, err)
		assert.EqualValues(t, 11, balance)

		if !useLegacyDeposits {
			accountRecords, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner)
			require.NoError(t, err)

			balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords[commonpb.AccountType_PRIMARY][0])
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 1)
			assert.EqualValues(t, 11, balanceByAccount[depositAccount.PublicKey().ToBase58()])

			balanceByAccount, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, depositAccount)
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 1)
			assert.EqualValues(t, 11, balanceByAccount[depositAccount.PublicKey().ToBase58()])
		}
	}
}

func TestDefaultCalculationMethods_MultipleIntents(t *testing.T) {
	for _, useLegacyIntents := range []bool{true, false} {
		env := setupBalanceTestEnv(t)

		owner1 := testutil.NewRandomAccount(t)
		a1, err := owner1.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		owner2 := testutil.NewRandomAccount(t)
		a2, err := owner2.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		owner3 := testutil.NewRandomAccount(t)
		a3, err := owner3.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		owner4 := testutil.NewRandomAccount(t)
		a4, err := owner4.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		externalAccount := testutil.NewRandomAccount(t)

		data := &balanceTestData{
			codeUsers: []*common.Account{owner1, owner2, owner3, owner4},
			transactions: []balanceTestTransaction{
				// Fund account a1 through a4 with an external deposit
				{source: externalAccount, destination: a1, quantity: 1, transactionState: transaction.ConfirmationFinalized},
				{source: externalAccount, destination: a2, quantity: 10, transactionState: transaction.ConfirmationFinalized},
				{source: externalAccount, destination: a3, quantity: 100, transactionState: transaction.ConfirmationFinalized},
				{source: externalAccount, destination: a4, quantity: 1000, transactionState: transaction.ConfirmationFinalized},
				// Confirmed intents are incorporated into balance calculations
				{source: a4, destination: a1, quantity: 1, intentID: "i1", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				{source: a4, destination: a1, quantity: 2, intentID: "i2", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				// Pending intents are incorporated into balance calculations
				{source: a4, destination: a2, quantity: 3, intentID: "i3", intentState: intent.StatePending, actionState: action.StatePending},
				{source: a4, destination: a2, quantity: 4, intentID: "i4", intentState: intent.StatePending, actionState: action.StatePending},
				// Failed intents are incorporated into balance calculations. We'll
				// always make the user whole.
				{source: a4, destination: a3, quantity: 5, intentID: "i5", intentState: intent.StateFailed, actionState: action.StateFailed},
				{source: a4, destination: a3, quantity: 6, intentID: "i6", intentState: intent.StateFailed, actionState: action.StateFailed},
				// Intents in the unknown state are incorporated differently depending
				// on the intent type, since it infers which intent system it came from.
				// Legacy intents are not incorporated, as the intent is not committed by
				// the client. Intents could theoretically by in the unknown state under
				// the new system, but we should limit this as much as possible.
				{source: a4, destination: a1, quantity: 7, intentID: "i7", intentState: intent.StateUnknown, actionState: action.StateUnknown},
				// Revoked intents are not incorporated into balance calculations.
				{source: a4, destination: a2, quantity: 8, intentID: "i8", intentState: intent.StateRevoked, actionState: action.StateRevoked},
			},
		}

		setupBalanceTestData(t, env, data, balanceTestDataConf{
			useLegacyIntents:  useLegacyIntents,
			useLegacyDeposits: useLegacyIntents,
		})

		balance, err := DefaultCalculation(env.ctx, env.data, a1)
		require.NoError(t, err)
		if useLegacyIntents {
			assert.EqualValues(t, 4, balance)
		} else {
			assert.EqualValues(t, 11, balance)
		}

		balance, err = DefaultCalculation(env.ctx, env.data, a2)
		require.NoError(t, err)
		assert.EqualValues(t, 17, balance)

		balance, err = DefaultCalculation(env.ctx, env.data, a3)
		require.NoError(t, err)
		assert.EqualValues(t, 111, balance)

		balance, err = DefaultCalculation(env.ctx, env.data, a4)
		require.NoError(t, err)
		if useLegacyIntents {
			assert.EqualValues(t, 979, balance)
		} else {
			assert.EqualValues(t, 972, balance)
		}

		if !useLegacyIntents {
			accountRecords1, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner1)
			require.NoError(t, err)

			accountRecords2, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner2)
			require.NoError(t, err)

			accountRecords3, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner3)
			require.NoError(t, err)

			accountRecords4, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner4)
			require.NoError(t, err)

			balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords1[commonpb.AccountType_PRIMARY][0], accountRecords2[commonpb.AccountType_PRIMARY][0], accountRecords3[commonpb.AccountType_PRIMARY][0], accountRecords4[commonpb.AccountType_PRIMARY][0])
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 4)
			assert.EqualValues(t, 11, balanceByAccount[a1.PublicKey().ToBase58()])
			assert.EqualValues(t, 17, balanceByAccount[a2.PublicKey().ToBase58()])
			assert.EqualValues(t, 111, balanceByAccount[a3.PublicKey().ToBase58()])
			assert.EqualValues(t, 972, balanceByAccount[a4.PublicKey().ToBase58()])

			balanceByAccount, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, a1, a2, a3, a4)
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 4)
			assert.EqualValues(t, 11, balanceByAccount[a1.PublicKey().ToBase58()])
			assert.EqualValues(t, 17, balanceByAccount[a2.PublicKey().ToBase58()])
			assert.EqualValues(t, 111, balanceByAccount[a3.PublicKey().ToBase58()])
			assert.EqualValues(t, 972, balanceByAccount[a4.PublicKey().ToBase58()])
		}
	}
}

func TestDefaultCalculationMethods_BackAndForth(t *testing.T) {
	for _, useLegacyIntents := range []bool{true, false} {
		env := setupBalanceTestEnv(t)

		owner1 := testutil.NewRandomAccount(t)
		a1, err := owner1.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		owner2 := testutil.NewRandomAccount(t)
		a2, err := owner2.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		externalAccount := testutil.NewRandomAccount(t)

		data := &balanceTestData{
			codeUsers: []*common.Account{owner1, owner2},
			transactions: []balanceTestTransaction{
				// Fund account a1 through an external deposit
				{source: externalAccount, destination: a1, quantity: 1, transactionState: transaction.ConfirmationFinalized},
				// Setup a set of intents that result in back and forth movement of the Kin
				{source: a1, destination: a2, quantity: 1, intentID: "i1", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				{source: a2, destination: a1, quantity: 1, intentID: "i2", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				{source: a1, destination: a2, quantity: 1, intentID: "i3", intentState: intent.StatePending, actionState: action.StatePending},
				{source: a2, destination: a1, quantity: 1, intentID: "i4", intentState: intent.StatePending, actionState: action.StatePending},
				{source: a1, destination: a2, quantity: 1, intentID: "i5", intentState: intent.StatePending, actionState: action.StatePending},
			},
		}

		setupBalanceTestData(t, env, data, balanceTestDataConf{
			useLegacyIntents:  useLegacyIntents,
			useLegacyDeposits: useLegacyIntents,
		})

		balance, err := DefaultCalculation(env.ctx, env.data, a1)
		require.NoError(t, err)
		assert.EqualValues(t, 0, balance)

		balance, err = DefaultCalculation(env.ctx, env.data, a2)
		require.NoError(t, err)
		assert.EqualValues(t, 1, balance)

		if !useLegacyIntents {
			accountRecords1, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner1)
			require.NoError(t, err)

			accountRecords2, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, owner2)
			require.NoError(t, err)

			balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords1[commonpb.AccountType_PRIMARY][0], accountRecords2[commonpb.AccountType_PRIMARY][0])
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 2)
			assert.EqualValues(t, 0, balanceByAccount[a1.PublicKey().ToBase58()])
			assert.EqualValues(t, 1, balanceByAccount[a2.PublicKey().ToBase58()])

			balanceByAccount, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, a1, a2)
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 2)
			assert.EqualValues(t, 0, balanceByAccount[a1.PublicKey().ToBase58()])
			assert.EqualValues(t, 1, balanceByAccount[a2.PublicKey().ToBase58()])
		}
	}
}

func TestDefaultCalculationMethods_SelfPayments(t *testing.T) {
	for _, useLegacyIntents := range []bool{true, false} {
		env := setupBalanceTestEnv(t)

		ownerAccount := testutil.NewRandomAccount(t)
		tokenAccount, err := ownerAccount.ToTimelockVault(getTimelockDataVersion(useLegacyIntents))
		require.NoError(t, err)

		externalAccount := testutil.NewRandomAccount(t)

		data := &balanceTestData{
			codeUsers: []*common.Account{ownerAccount},
			transactions: []balanceTestTransaction{
				// Fund account the token account through an external deposit
				{source: externalAccount, destination: tokenAccount, quantity: 1, transactionState: transaction.ConfirmationFinalized},
				// Setup a set of intents that result in self-payments and no-ops to
				// the balance calculation
				{source: tokenAccount, destination: tokenAccount, quantity: 1, intentID: "i1", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				{source: tokenAccount, destination: tokenAccount, quantity: 1, intentID: "i2", intentState: intent.StateConfirmed, actionState: action.StateConfirmed, transactionState: transaction.ConfirmationFinalized},
				{source: tokenAccount, destination: tokenAccount, quantity: 1, intentID: "i3", intentState: intent.StatePending, actionState: action.StatePending},
				{source: tokenAccount, destination: tokenAccount, quantity: 1, intentID: "i4", intentState: intent.StatePending, actionState: action.StatePending},
			},
		}

		setupBalanceTestData(t, env, data, balanceTestDataConf{
			useLegacyIntents:  useLegacyIntents,
			useLegacyDeposits: useLegacyIntents,
		})

		balance, err := DefaultCalculation(env.ctx, env.data, tokenAccount)
		require.NoError(t, err)
		assert.EqualValues(t, 1, balance)

		if !useLegacyIntents {
			accountRecords, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, ownerAccount)
			require.NoError(t, err)

			balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords[commonpb.AccountType_PRIMARY][0])
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 1)
			assert.EqualValues(t, 1, balanceByAccount[tokenAccount.PublicKey().ToBase58()])

			balanceByAccount, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, tokenAccount)
			require.NoError(t, err)
			require.Len(t, balanceByAccount, 1)
			assert.EqualValues(t, 1, balanceByAccount[tokenAccount.PublicKey().ToBase58()])
		}
	}
}

func TestDefaultCalculationMethods_NotManagedByCode(t *testing.T) {
	env := setupBalanceTestEnv(t)

	ownerAccount := testutil.NewRandomAccount(t)
	tokenAccount, err := ownerAccount.ToTimelockVault(getTimelockDataVersion(false))
	require.NoError(t, err)

	data := &balanceTestData{
		codeUsers: []*common.Account{ownerAccount},
	}

	setupBalanceTestData(t, env, data, balanceTestDataConf{})

	timelockRecord, err := env.data.GetTimelockByVault(env.ctx, tokenAccount.PublicKey().ToBase58())
	require.NoError(t, err)
	timelockRecord.VaultState = timelock_token_v1.StateWaitingForTimeout
	timelockRecord.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	accountRecords, err := common.GetLatestTokenAccountRecordsForOwner(env.ctx, env.data, ownerAccount)
	require.NoError(t, err)

	_, err = DefaultCalculation(env.ctx, env.data, tokenAccount)
	assert.Equal(t, ErrNotManagedByCode, err)

	_, err = DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, accountRecords[commonpb.AccountType_PRIMARY][0])
	assert.Equal(t, ErrNotManagedByCode, err)

	_, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, tokenAccount)
	assert.Equal(t, ErrNotManagedByCode, err)
}

func TestDefaultBatchCalculation_PrePrivacyAccounts(t *testing.T) {
	env := setupBalanceTestEnv(t)

	ownerAccount := testutil.NewRandomAccount(t)
	legacyTokenAccount, err := ownerAccount.ToTimelockVault(getTimelockDataVersion(true))
	require.NoError(t, err)

	data := &balanceTestData{
		codeUsers: []*common.Account{ownerAccount},
	}

	setupBalanceTestData(t, env, data, balanceTestDataConf{
		useLegacyIntents: true,
	})

	timelockRecord, err := env.data.GetTimelockByVault(env.ctx, legacyTokenAccount.PublicKey().ToBase58())
	require.NoError(t, err)

	_, err = DefaultBatchCalculationWithAccountRecords(env.ctx, env.data, &common.AccountRecords{Timelock: timelockRecord})
	assert.Equal(t, ErrUnhandledAccount, err)

	_, err = DefaultBatchCalculationWithTokenAccounts(env.ctx, env.data, legacyTokenAccount)
	assert.Equal(t, ErrUnhandledAccount, err)
}

func TestDefaultCalculation_ExternalAccount(t *testing.T) {
	env := setupBalanceTestEnv(t)
	externalAccount := testutil.NewRandomAccount(t)
	_, err := DefaultCalculation(env.ctx, env.data, externalAccount)
	assert.Equal(t, ErrNotManagedByCode, err)

	// Note: not possible with batch method, since we wouldn't have account records
}

func TestGetAggregatedBalances(t *testing.T) {
	env := setupBalanceTestEnv(t)

	owner := testutil.NewRandomAccount(t)

	_, err := GetTotalBalance(env.ctx, env.data, owner)
	assert.Equal(t, ErrNotManagedByCode, err)

	_, err = GetPrivateBalance(env.ctx, env.data, owner)
	assert.Equal(t, ErrNotManagedByCode, err)

	var expectedTotalBalance, expectedPrivateBalance uint64
	for i, accountType := range account.AllAccountTypes {
		if accountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
			continue
		}

		authority := testutil.NewRandomAccount(t)
		if accountType == commonpb.AccountType_PRIMARY {
			authority = owner
		}

		balance := uint64(math.Pow10(i))
		expectedTotalBalance += balance
		if accountType != commonpb.AccountType_PRIMARY && accountType != commonpb.AccountType_RELATIONSHIP {
			expectedPrivateBalance += balance
		}

		timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1)
		require.NoError(t, err)

		timelockRecord := timelockAccounts.ToDBRecord()
		require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

		accountInfoRecord := account.Record{
			OwnerAccount:     owner.PublicKey().ToBase58(),
			AuthorityAccount: authority.PublicKey().ToBase58(),
			TokenAccount:     timelockRecord.VaultAddress,
			AccountType:      accountType,
		}
		if accountType == commonpb.AccountType_RELATIONSHIP {
			accountInfoRecord.RelationshipTo = pointer.String("example.com")
		}
		require.NoError(t, env.data.CreateAccountInfo(env.ctx, &accountInfoRecord))

		actionRecord := action.Record{
			Intent:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType:  intent.SendPrivatePayment,
			ActionType:  action.PrivateTransfer,
			Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Destination: &accountInfoRecord.TokenAccount,
			Quantity:    &balance,
		}
		require.NoError(t, env.data.PutAllActions(env.ctx, &actionRecord))
	}

	balance, err := GetTotalBalance(env.ctx, env.data, owner)
	require.NoError(t, err)
	assert.EqualValues(t, expectedTotalBalance, balance)

	balance, err = GetPrivateBalance(env.ctx, env.data, owner)
	require.NoError(t, err)
	assert.EqualValues(t, expectedPrivateBalance, balance)
}

type balanceTestEnv struct {
	ctx  context.Context
	data code_data.Provider
}

type balanceTestData struct {
	codeUsers    []*common.Account
	transactions []balanceTestTransaction
}

type balanceTestTransaction struct {
	source, destination *common.Account
	quantity            uint64

	intentID    string
	intentState intent.State
	actionState action.State

	transactionState transaction.Confirmation
}

func setupBalanceTestEnv(t *testing.T) (env balanceTestEnv) {
	env.ctx = context.Background()
	env.data = code_data.NewTestDataProvider()
	testutil.SetupRandomSubsidizer(t, env.data)
	return env
}

type balanceTestDataConf struct {
	useLegacyIntents  bool
	useLegacyDeposits bool
}

func setupBalanceTestData(t *testing.T, env balanceTestEnv, data *balanceTestData, conf balanceTestDataConf) {
	for _, owner := range data.codeUsers {
		timelockAccounts, err := owner.GetTimelockAccounts(getTimelockDataVersion(conf.useLegacyIntents))
		require.NoError(t, err)
		timelockRecord := timelockAccounts.ToDBRecord()
		timelockRecord.VaultState = timelock_token_v1.StateLocked
		timelockRecord.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

		if !conf.useLegacyIntents {
			accountInfoRecord := &account.Record{
				OwnerAccount:     owner.PublicKey().ToBase58(),
				AuthorityAccount: owner.PublicKey().ToBase58(),
				TokenAccount:     timelockRecord.VaultAddress,
				AccountType:      commonpb.AccountType_PRIMARY,
			}
			require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))
		}
	}

	for i, txn := range data.transactions {
		// Setup the intent record with an equivalent action record
		if len(txn.intentID) > 0 {
			if conf.useLegacyIntents {
				intentRecord := &intent.Record{
					IntentId:              txn.intentID,
					IntentType:            intent.LegacyPayment,
					InitiatorOwnerAccount: "owner",
					MoneyTransferMetadata: &intent.MoneyTransferMetadata{
						Source:      txn.source.PublicKey().ToBase58(),
						Destination: txn.destination.PublicKey().ToBase58(),
						Quantity:    txn.quantity,

						ExchangeCurrency: currency.KIN,
						ExchangeRate:     1.0,
						UsdMarketValue:   1.0,
					},
					State:     txn.intentState,
					CreatedAt: time.Now(),
				}
				require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
			} else {
				intentRecord := &intent.Record{
					IntentId:              txn.intentID,
					IntentType:            intent.SendPrivatePayment,
					InitiatorOwnerAccount: "owner",
					SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
						DestinationOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
						DestinationTokenAccount: txn.destination.PublicKey().ToBase58(),
						Quantity:                txn.quantity,

						ExchangeCurrency: currency.KIN,
						ExchangeRate:     1.0,
						NativeAmount:     1.0,
						UsdMarketValue:   1.0,
					},
					State:     txn.intentState,
					CreatedAt: time.Now(),
				}
				require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))

				actionRecord := &action.Record{
					Intent:     txn.intentID,
					IntentType: intent.SendPrivatePayment,

					ActionId:   0,
					ActionType: action.PrivateTransfer,

					Source:      txn.source.PublicKey().ToBase58(),
					Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
					Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,

					State: txn.actionState,
				}
				require.NoError(t, env.data.PutAllActions(env.ctx, actionRecord))
			}
		}

		// We have an intent, and it's confirmed, so a payment record exists
		if len(txn.intentID) > 0 && txn.intentState == intent.StateConfirmed {
			paymentRecord := &payment.Record{
				Source:      txn.source.PublicKey().ToBase58(),
				Destination: txn.destination.PublicKey().ToBase58(),
				Quantity:    txn.quantity,

				Rendezvous: txn.intentID,
				IsExternal: false,

				TransactionId: fmt.Sprintf("txn%d", i),

				ConfirmationState: txn.transactionState,

				// Below fields are irrelevant and can be set to whatever
				ExchangeCurrency: string(currency.KIN),
				ExchangeRate:     1.0,
				UsdMarketValue:   1.0,

				BlockId: 12345,

				CreatedAt: time.Now(),
			}
			require.NoError(t, env.data.CreatePayment(env.ctx, paymentRecord))
		}

		// There's no intent, so we have an external deposit. Depending on legacy
		// status, we either have a legacy payment record or a new deposit record.
		//
		// todo: configuration for legacy deposits
		if len(txn.intentID) == 0 {
			if conf.useLegacyDeposits {
				paymentRecord := &payment.Record{
					Source:      txn.source.PublicKey().ToBase58(),
					Destination: txn.destination.PublicKey().ToBase58(),
					Quantity:    txn.quantity,

					Rendezvous: "",
					IsExternal: true,

					TransactionId: fmt.Sprintf("txn%d", i),

					ConfirmationState: txn.transactionState,

					// Below fields are irrelevant and can be set to whatever
					ExchangeCurrency: string(currency.KIN),
					ExchangeRate:     1.0,
					UsdMarketValue:   1.0,

					BlockId: 12345,

					CreatedAt: time.Now(),
				}
				require.NoError(t, env.data.CreatePayment(env.ctx, paymentRecord))
			}

			if !conf.useLegacyDeposits && txn.transactionState != transaction.ConfirmationUnknown {
				depositRecord := &deposit.Record{
					Signature:      fmt.Sprintf("txn%d", i),
					Destination:    txn.destination.PublicKey().ToBase58(),
					Amount:         txn.quantity,
					UsdMarketValue: 1.0,

					Slot:              12345,
					ConfirmationState: txn.transactionState,

					CreatedAt: time.Now(),
				}
				require.NoError(t, env.data.SaveExternalDeposit(env.ctx, depositRecord))
			}
		}
	}
}

func getTimelockDataVersion(useLegacyIntents bool) timelock_token_v1.TimelockDataVersion {
	if useLegacyIntents {
		return timelock_token_v1.DataVersionLegacy
	}
	return timelock_token_v1.DataVersion1
}
