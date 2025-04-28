package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/currency"
)

func RunTests(t *testing.T, s intent.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s intent.Store){
		testOpenAccountsRoundTrip,
		testExternalDepositRoundTrip,
		testSendPublicPaymentRoundTrip,
		testReceivePaymentsPubliclyRoundTrip,
		testUpdate,
		testGetLatestByInitiatorAndType,
		testGetOriginalGiftCardIssuedIntent,
		testGetGiftCardClaimedIntent,
	} {
		tf(t, s)
		teardown()
	}
}

func testOpenAccountsRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testOpenAccountsRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.OpenAccounts,
			InitiatorOwnerAccount: "test_owner",
			OpenAccountsMetadata:  &intent.OpenAccountsMetadata{},
			ExtendedMetadata:      []byte("extended_metadata"),
			State:                 intent.StateUnknown,
			CreatedAt:             time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		require.NotNil(t, actual.OpenAccountsMetadata)
		assert.Equal(t, cloned.ExtendedMetadata, actual.ExtendedMetadata)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testExternalDepositRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testExternalDepositRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.ExternalDeposit,
			InitiatorOwnerAccount: "test_owner",
			ExternalDepositMetadata: &intent.ExternalDepositMetadata{
				DestinationOwnerAccount: "test_destination_owner",
				DestinationTokenAccount: "test_destination_token",
				Quantity:                12345,
				UsdMarketValue:          1.2345,
			},
			ExtendedMetadata: []byte("extended_metadata"),
			State:            intent.StateUnknown,
			CreatedAt:        time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		require.NotNil(t, actual.ExternalDepositMetadata)
		assert.Equal(t, cloned.ExternalDepositMetadata.DestinationOwnerAccount, actual.ExternalDepositMetadata.DestinationOwnerAccount)
		assert.Equal(t, cloned.ExternalDepositMetadata.DestinationTokenAccount, actual.ExternalDepositMetadata.DestinationTokenAccount)
		assert.Equal(t, cloned.ExternalDepositMetadata.Quantity, actual.ExternalDepositMetadata.Quantity)
		assert.Equal(t, cloned.ExternalDepositMetadata.UsdMarketValue, actual.ExternalDepositMetadata.UsdMarketValue)
		assert.Equal(t, cloned.ExtendedMetadata, actual.ExtendedMetadata)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testSendPublicPaymentRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testSendPublicPaymentRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.SendPublicPayment,
			InitiatorOwnerAccount: "test_owner",
			SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
				DestinationOwnerAccount: "test_destination_owner",
				DestinationTokenAccount: "test_destination_token",
				Quantity:                12345,

				ExchangeCurrency: currency.CAD,
				ExchangeRate:     0.00073,
				NativeAmount:     0.00073 * 12345,
				UsdMarketValue:   0.00042,

				IsWithdrawal: true,
				IsRemoteSend: true,
			},
			ExtendedMetadata: []byte("extended_metadata"),
			State:            intent.StateUnknown,
			CreatedAt:        time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		require.NotNil(t, actual.SendPublicPaymentMetadata)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.DestinationOwnerAccount, actual.SendPublicPaymentMetadata.DestinationOwnerAccount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.DestinationTokenAccount, actual.SendPublicPaymentMetadata.DestinationTokenAccount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.Quantity, actual.SendPublicPaymentMetadata.Quantity)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.ExchangeCurrency, actual.SendPublicPaymentMetadata.ExchangeCurrency)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.ExchangeRate, actual.SendPublicPaymentMetadata.ExchangeRate)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.NativeAmount, actual.SendPublicPaymentMetadata.NativeAmount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.UsdMarketValue, actual.SendPublicPaymentMetadata.UsdMarketValue)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.IsWithdrawal, actual.SendPublicPaymentMetadata.IsWithdrawal)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.IsRemoteSend, actual.SendPublicPaymentMetadata.IsRemoteSend)
		assert.Equal(t, cloned.ExtendedMetadata, actual.ExtendedMetadata)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testReceivePaymentsPubliclyRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testReceivePaymentsPubliclyRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.ReceivePaymentsPublicly,
			InitiatorOwnerAccount: "test_owner",
			ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
				Source:                   "test_source",
				Quantity:                 12345,
				IsRemoteSend:             true,
				IsReturned:               true,
				IsIssuerVoidingGiftCard:  true,
				OriginalExchangeCurrency: "usd",
				OriginalExchangeRate:     0.1,
				OriginalNativeAmount:     1234.5,
				UsdMarketValue:           999.99,
			},
			ExtendedMetadata: []byte("extended_metadata"),
			State:            intent.StateUnknown,
			CreatedAt:        time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		require.NotNil(t, actual.ReceivePaymentsPubliclyMetadata)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.Source, actual.ReceivePaymentsPubliclyMetadata.Source)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.Quantity, actual.ReceivePaymentsPubliclyMetadata.Quantity)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.IsRemoteSend, actual.ReceivePaymentsPubliclyMetadata.IsRemoteSend)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.IsReturned, actual.ReceivePaymentsPubliclyMetadata.IsReturned)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard, actual.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency, actual.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate, actual.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount, actual.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount)
		assert.Equal(t, cloned.ReceivePaymentsPubliclyMetadata.UsdMarketValue, actual.ReceivePaymentsPubliclyMetadata.UsdMarketValue)
		assert.Equal(t, cloned.ExtendedMetadata, actual.ExtendedMetadata)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testUpdate(t *testing.T, s intent.Store) {
	t.Run("testUpdate", func(t *testing.T) {
		ctx := context.Background()

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.OpenAccounts,
			InitiatorOwnerAccount: "test_owner",
			OpenAccountsMetadata:  &intent.OpenAccountsMetadata{},
			State:                 intent.StateUnknown,
			CreatedAt:             time.Now(),
		}
		err := s.Save(ctx, &expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)

		expected.State = intent.StatePending

		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err := s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, intent.StatePending, actual.State)
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testGetLatestByInitiatorAndType(t *testing.T, s intent.Store) {
	ctx := context.Background()

	t.Run("testGetLatestByInitiatorAndType", func(t *testing.T) {
		records := []intent.Record{
			{IntentId: "t1", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o1", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StatePending},
			{IntentId: "t2", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o1", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StateFailed},
			{IntentId: "t3", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o1", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StateUnknown},
			{IntentId: "t4", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o1", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StateUnknown},
			{IntentId: "t5", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o2", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StateUnknown},
			{IntentId: "t6", IntentType: intent.OpenAccounts, InitiatorOwnerAccount: "o2", OpenAccountsMetadata: &intent.OpenAccountsMetadata{}, State: intent.StateFailed},
		}
		for i, record := range records {
			record.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
			require.NoError(t, s.Save(ctx, &record))
		}

		_, err := s.GetLatestByInitiatorAndType(ctx, intent.SendPublicPayment, "o1")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		actual, err := s.GetLatestByInitiatorAndType(ctx, intent.OpenAccounts, "o1")
		require.NoError(t, err)
		assert.Equal(t, "t4", actual.IntentId)
	})

}

func testGetOriginalGiftCardIssuedIntent(t *testing.T, s intent.Store) {
	t.Run("testGetOriginalGiftCardIssuedIntent", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: false, DestinationTokenAccount: "a1", DestinationOwnerAccount: "o1", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i2", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i3", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: false, DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i4", IntentType: intent.ExternalDeposit, ExternalDepositMetadata: &intent.ExternalDepositMetadata{DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i5", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a3", DestinationOwnerAccount: "o3", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i6", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a3", DestinationOwnerAccount: "o3", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i7", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a4", DestinationOwnerAccount: "o4", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StatePending},
			{IntentId: "i8", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a4", DestinationOwnerAccount: "o4", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateRevoked},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		_, err := s.GetOriginalGiftCardIssuedIntent(ctx, "unknown")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		_, err = s.GetOriginalGiftCardIssuedIntent(ctx, "a1")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		actual, err := s.GetOriginalGiftCardIssuedIntent(ctx, "a2")
		require.NoError(t, err)
		assert.Equal(t, "i2", actual.IntentId)

		_, err = s.GetOriginalGiftCardIssuedIntent(ctx, "a3")
		assert.Equal(t, intent.ErrMultilpeIntentsFound, err)

		actual, err = s.GetOriginalGiftCardIssuedIntent(ctx, "a4")
		require.NoError(t, err)
		assert.Equal(t, "i7", actual.IntentId)
	})
}

func testGetGiftCardClaimedIntent(t *testing.T, s intent.Store) {
	t.Run("testGetGiftCardClaimedIntent", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: false, Source: "a1", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i2", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: false, Source: "a2", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i3", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a2", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i4", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a3", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i5", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a3", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i6", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a4", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateRevoked},
			{IntentId: "i7", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a4", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StatePending},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		_, err := s.GetGiftCardClaimedIntent(ctx, "unknown")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		_, err = s.GetGiftCardClaimedIntent(ctx, "a1")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		actual, err := s.GetGiftCardClaimedIntent(ctx, "a2")
		require.NoError(t, err)
		assert.Equal(t, "i3", actual.IntentId)

		_, err = s.GetGiftCardClaimedIntent(ctx, "a3")
		assert.Equal(t, intent.ErrMultilpeIntentsFound, err)

		actual, err = s.GetGiftCardClaimedIntent(ctx, "a4")
		require.NoError(t, err)
		assert.Equal(t, "i7", actual.IntentId)
	})
}
