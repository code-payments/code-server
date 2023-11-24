package lawenforcement

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
)

func TestAntiMoneyLaunderingGuard_SendPrivatePayment_TransactionValue(t *testing.T) {
	env := setupAmlTest(t)

	phoneNumber := "+12223334444"
	owner := testutil.NewRandomAccount(t)
	setupPhoneUser(t, env, phoneNumber)

	for _, acceptableValue := range []float64{
		1,
		maxUsdTransactionValue / 10,
		maxUsdTransactionValue - 1,
		maxUsdTransactionValue,
	} {
		intentRecord := makeSendPrivatePaymentIntent(t, phoneNumber, owner, acceptableValue, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}

	for _, unacceptableValue := range []float64{
		maxUsdTransactionValue + 1,
		maxUsdTransactionValue * 10,
	} {
		intentRecord := makeSendPrivatePaymentIntent(t, phoneNumber, owner, unacceptableValue, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.False(t, allow)
	}
}

func TestAntiMoneyLaunderingGuard_SendPrivatePayment_DailyUsdLimit(t *testing.T) {
	env := setupAmlTest(t)

	for i, tc := range []struct {
		consumedUsdValue float64
		at               time.Time
		expected         bool
	}{
		// Intent consumes some of the daily limit, but not all
		{
			consumedUsdValue: maxDailyUsdLimit / 2,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         true,
		},
		// Intent consumes the remaining daily limit
		{
			consumedUsdValue: maxDailyUsdLimit - 1,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         true,
		},
		// Daily limit was breached, but more than a day ago
		{
			consumedUsdValue: maxDailyUsdLimit + 1,
			at:               time.Now().Add(-24*time.Hour - time.Minute),
			expected:         true,
		},
		// Daily limit is breached, but is close to expiring
		{
			consumedUsdValue: maxDailyUsdLimit,
			at:               time.Now().Add(-24*time.Hour + time.Minute),
			expected:         false,
		},
		// Daily limit is breached well within the time window
		{
			consumedUsdValue: maxDailyUsdLimit + 1,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         false,
		},
	} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)
		owner := testutil.NewRandomAccount(t)
		setupPhoneUser(t, env, phoneNumber)
		intentRecord := makeSendPrivatePaymentIntent(t, phoneNumber, owner, 1, time.Now())

		// Sanity check the intent for $1 USD is allowed
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)

		// Save an intent to bring the user up to the desired consumed daily USD value
		require.NoError(t, env.data.SaveIntent(env.ctx, makeSendPrivatePaymentIntent(t, phoneNumber, owner, tc.consumedUsdValue, tc.at)))

		// Check whether we allow the $1 USD intent
		allow, err = env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, allow)
	}
}

func TestAntiMoneyLaunderingGuard_ReceivePaymentsPrivately_Deposit_TransactionValue(t *testing.T) {
	env := setupAmlTest(t)

	phoneNumber := "+12223334444"
	owner := testutil.NewRandomAccount(t)
	setupPhoneUser(t, env, phoneNumber)
	setupPrivateBalance(t, env, owner, 0)

	for _, acceptableValue := range []float64{
		1,
		maxUsdTransactionValue / 10,
		maxUsdTransactionValue - 1,
		maxUsdTransactionValue,
	} {
		intentRecord := makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, acceptableValue, true, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}

	for _, unacceptableValue := range []float64{
		maxUsdTransactionValue + 1,
		maxUsdTransactionValue * 10,
	} {
		intentRecord := makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, unacceptableValue, true, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.False(t, allow)
	}
}

func TestAntiMoneyLaunderingGuard_ReceivePaymentsPrivately_Deposit_DailyUsdLimit(t *testing.T) {
	env := setupAmlTest(t)

	for i, tc := range []struct {
		consumedUsdValue float64
		at               time.Time
		expected         bool
	}{
		// Intent consumes some of the daily limit, but not all
		{
			consumedUsdValue: maxDailyUsdLimit / 2,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         true,
		},
		// Intent consumes the remaining daily limit
		{
			consumedUsdValue: maxDailyUsdLimit - 1,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         true,
		},
		// Daily limit was breached, but more than a day ago
		{
			consumedUsdValue: maxDailyUsdLimit + 1,
			at:               time.Now().Add(-24*time.Hour - time.Minute),
			expected:         true,
		},
		// Daily limit is breached, but is close to expiring
		{
			consumedUsdValue: maxDailyUsdLimit,
			at:               time.Now().Add(-24*time.Hour + time.Minute),
			expected:         false,
		},
		// Daily limit is breached well within the time window
		{
			consumedUsdValue: maxDailyUsdLimit + 1,
			at:               time.Now().Add(-12 * time.Hour),
			expected:         false,
		},
	} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)
		owner := testutil.NewRandomAccount(t)
		setupPhoneUser(t, env, phoneNumber)
		setupPrivateBalance(t, env, owner, 0)

		intentRecord := makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, 1, true, time.Now())

		// Sanity check the intent for $1 USD is allowed
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)

		// Save an intent to bring the user up to the desired consumed daily USD value
		require.NoError(t, env.data.SaveIntent(env.ctx, makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, tc.consumedUsdValue, true, tc.at)))

		// Check whether we allow the $1 USD intent
		allow, err = env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, allow)
	}
}

func TestAntiMoneyLaunderingGuard_ReceivePaymentsPrivately_Deposit_TotalPrivateBalance(t *testing.T) {
	env := setupAmlTest(t)

	usdExchangeRecord, err := env.data.GetExchangeRate(env.ctx, currency_lib.USD, time.Now())
	require.NoError(t, err)

	for i, tc := range []struct {
		balance  uint64
		expected bool
	}{
		{
			balance:  0,
			expected: true,
		},
		{
			balance:  1,
			expected: true,
		},
		{
			balance:  kin.ToQuarks(uint64(maxUsdPrivateBalance/usdExchangeRecord.Rate)) / 2,
			expected: true,
		},
		{
			balance:  kin.ToQuarks(uint64(maxUsdPrivateBalance/usdExchangeRecord.Rate)) - 1,
			expected: true,
		},
		{
			balance:  kin.ToQuarks(uint64(maxUsdPrivateBalance / usdExchangeRecord.Rate)),
			expected: false,
		},
		{
			balance:  kin.ToQuarks(uint64(maxUsdPrivateBalance/usdExchangeRecord.Rate)) + 1,
			expected: false,
		},
	} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)
		owner := testutil.NewRandomAccount(t)
		setupPhoneUser(t, env, phoneNumber)
		setupPrivateBalance(t, env, owner, tc.balance)

		intentRecord := makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, 1, true, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, allow)
	}
}

func TestAntiMoneyLaunderingGuard_ReceivePaymentsPrivately_CodeToCodePayment(t *testing.T) {
	env := setupAmlTest(t)

	phoneNumber := "+12223334444"
	owner := testutil.NewRandomAccount(t)
	setupPhoneUser(t, env, phoneNumber)

	// Create something that will blow away the daily limit
	require.NoError(t, env.data.SaveIntent(env.ctx, makeSendPrivatePaymentIntent(t, phoneNumber, owner, 2*maxDailyUsdLimit, time.Now())))

	for _, usdMarketValue := range []float64{
		1,
		1_000_000_000_000,
	} {
		intentRecord := makeReceivePaymentsPrivatelyIntent(t, phoneNumber, owner, usdMarketValue, false, time.Now())

		// We should always allow a receive payment intent
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}
}

func TestAntiMoneyLaunderingGuard_SendPublicPayment(t *testing.T) {
	env := setupAmlTest(t)

	phoneNumber := "+12223334444"
	owner := testutil.NewRandomAccount(t)
	setupPhoneUser(t, env, phoneNumber)

	// Create something that will blow away the daily limit
	require.NoError(t, env.data.SaveIntent(env.ctx, makeSendPrivatePaymentIntent(t, phoneNumber, owner, 2*maxDailyUsdLimit, time.Now())))

	for _, usdMarketValue := range []float64{
		1,
		1_000_000_000_000,
	} {
		intentRecord := makeSendPublicPaymentIntent(t, phoneNumber, owner, usdMarketValue, time.Now())

		// We should always allow a public payment
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}
}

func TestAntiMoneyLaunderingGuard_ReceivePaymentsPublicly(t *testing.T) {
	env := setupAmlTest(t)

	phoneNumber := "+12223334444"
	owner := testutil.NewRandomAccount(t)
	setupPhoneUser(t, env, phoneNumber)

	// Create something that will blow away the daily limit
	require.NoError(t, env.data.SaveIntent(env.ctx, makeReceivePaymentsPubliclyIntent(t, phoneNumber, owner, 2*maxDailyUsdLimit, time.Now())))

	for _, usdMarketValue := range []float64{
		1,
		1_000_000_000_000,
	} {
		intentRecord := makeSendPublicPaymentIntent(t, phoneNumber, owner, usdMarketValue, time.Now())

		// We should always allow a public payment
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}
}

type amlTestEnv struct {
	ctx   context.Context
	data  code_data.Provider
	guard *AntiMoneyLaunderingGuard
}

func setupAmlTest(t *testing.T) (env amlTestEnv) {
	env.ctx = context.Background()
	env.data = code_data.NewTestDataProvider()
	env.guard = NewAntiMoneyLaunderingGuard(env.data)

	testutil.SetupRandomSubsidizer(t, env.data)

	env.data.ImportExchangeRates(env.ctx, &currency.MultiRateRecord{
		Time: time.Now(),
		Rates: map[string]float64{
			string(currency_lib.USD): 0.1,
		},
	})

	return env
}

func setupPhoneUser(t *testing.T, env amlTestEnv, phoneNumber string) {
	require.NoError(t, env.data.PutUser(env.ctx, &identity.Record{
		ID: user.NewUserID(),
		View: &user.View{
			PhoneNumber: &phoneNumber,
		},
		IsStaffUser: true,
		CreatedAt:   time.Now(),
	}))
}

func makeSendPrivatePaymentIntent(t *testing.T, phoneNumber string, owner *common.Account, usdMarketValue float64, at time.Time) *intent.Record {
	return &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.SendPrivatePayment,

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			DestinationOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Quantity:                uint64(usdMarketValue),

			ExchangeRate:     1,
			ExchangeCurrency: currency_lib.USD,
			NativeAmount:     usdMarketValue,
			UsdMarketValue:   usdMarketValue,
		},

		InitiatorOwnerAccount: owner.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &phoneNumber,

		State:     intent.StatePending,
		CreatedAt: at,
	}
}

func makeSendPublicPaymentIntent(t *testing.T, phoneNumber string, owner *common.Account, usdMarketValue float64, at time.Time) *intent.Record {
	return &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.SendPublicPayment,

		SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
			DestinationOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Quantity:                uint64(usdMarketValue),

			ExchangeRate:     1,
			ExchangeCurrency: currency_lib.USD,
			NativeAmount:     usdMarketValue,
			UsdMarketValue:   usdMarketValue,
		},

		InitiatorOwnerAccount: owner.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &phoneNumber,

		State:     intent.StatePending,
		CreatedAt: at,
	}
}

func makeReceivePaymentsPrivatelyIntent(t *testing.T, phoneNumber string, owner *common.Account, usdMarketValue float64, isDeposit bool, at time.Time) *intent.Record {
	return &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.ReceivePaymentsPrivately,

		ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{
			Source:    testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Quantity:  uint64(usdMarketValue),
			IsDeposit: isDeposit,

			UsdMarketValue: usdMarketValue,
		},

		InitiatorOwnerAccount: owner.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &phoneNumber,

		State:     intent.StatePending,
		CreatedAt: at,
	}
}

func makeReceivePaymentsPubliclyIntent(t *testing.T, phoneNumber string, owner *common.Account, usdMarketValue float64, at time.Time) *intent.Record {
	return &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.ReceivePaymentsPublicly,

		ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
			Source:       testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Quantity:     uint64(usdMarketValue),
			IsRemoteSend: true,

			OriginalExchangeCurrency: currency_lib.USD,
			OriginalExchangeRate:     1.0,
			OriginalNativeAmount:     usdMarketValue,

			UsdMarketValue: usdMarketValue,
		},

		InitiatorOwnerAccount: owner.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &phoneNumber,

		State:     intent.StatePending,
		CreatedAt: at,
	}
}

func setupPrivateBalance(t *testing.T, env amlTestEnv, owner *common.Account, balance uint64) {
	authority := testutil.NewRandomAccount(t)

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token.DataVersion1)
	require.NoError(t, err)

	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	accountInfoRecord := account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: authority.PublicKey().ToBase58(),
		TokenAccount:     timelockRecord.VaultAddress,
		AccountType:      commonpb.AccountType_BUCKET_1_KIN,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &accountInfoRecord))

	if balance > 0 {
		actionRecord := &action.Record{
			Intent:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType:  intent.SendPrivatePayment,
			ActionType:  action.PrivateTransfer,
			Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			Destination: &accountInfoRecord.TokenAccount,
			Quantity:    &balance,
		}
		require.NoError(t, env.data.PutAllActions(env.ctx, actionRecord))
	}
}
