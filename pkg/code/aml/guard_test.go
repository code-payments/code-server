package aml

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestGuard_SendPublicPayment_TransactionValue(t *testing.T) {
	env := setupAmlTest(t)

	owner := testutil.NewRandomAccount(t)

	for _, acceptableValue := range []float64{
		1,
		maxUsdTransactionValue / 10,
		maxUsdTransactionValue - 1,
		maxUsdTransactionValue,
	} {
		intentRecord := makeSendPublicPaymentIntent(t, owner, acceptableValue, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}

	for _, unacceptableValue := range []float64{
		maxUsdTransactionValue + 1,
		maxUsdTransactionValue * 10,
	} {
		intentRecord := makeSendPublicPaymentIntent(t, owner, unacceptableValue, time.Now())

		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.False(t, allow)
	}
}

func TestGuard_SendPublicPayment_DailyUsdLimit(t *testing.T) {
	env := setupAmlTest(t)

	for _, tc := range []struct {
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
		owner := testutil.NewRandomAccount(t)
		intentRecord := makeSendPublicPaymentIntent(t, owner, 1, time.Now())

		// Sanity check the intent for $1 USD is allowed
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)

		// Save an intent to bring the user up to the desired consumed daily USD value
		require.NoError(t, env.data.SaveIntent(env.ctx, makeSendPublicPaymentIntent(t, owner, tc.consumedUsdValue, tc.at)))

		// Check whether we allow the $1 USD intent
		allow, err = env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, allow)
	}
}

func TestGuard_ReceivePaymentsPublicly(t *testing.T) {
	env := setupAmlTest(t)

	owner := testutil.NewRandomAccount(t)

	for _, usdMarketValue := range []float64{
		1,
		1_000_000_000_000,
	} {
		intentRecord := makeReceivePaymentsPubliclyIntent(t, owner, usdMarketValue, time.Now())

		// We should always allow a public receive
		allow, err := env.guard.AllowMoneyMovement(env.ctx, intentRecord)
		require.NoError(t, err)
		assert.True(t, allow)
	}
}

type amlTestEnv struct {
	ctx   context.Context
	data  code_data.Provider
	guard *Guard
}

func setupAmlTest(t *testing.T) (env amlTestEnv) {
	env.ctx = context.Background()
	env.data = code_data.NewTestDataProvider()
	env.guard = NewGuard(env.data)

	testutil.SetupRandomSubsidizer(t, env.data)

	env.data.ImportExchangeRates(env.ctx, &currency.MultiRateRecord{
		Time: time.Now(),
		Rates: map[string]float64{
			string(currency_lib.USD): 0.1,
		},
	})

	return env
}

func makeSendPublicPaymentIntent(t *testing.T, owner *common.Account, usdMarketValue float64, at time.Time) *intent.Record {
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

		State:     intent.StatePending,
		CreatedAt: at,
	}
}

func makeReceivePaymentsPubliclyIntent(t *testing.T, owner *common.Account, usdMarketValue float64, at time.Time) *intent.Record {
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

		State:     intent.StatePending,
		CreatedAt: at,
	}
}
