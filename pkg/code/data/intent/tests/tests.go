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
		testLegacyPaymentRoundTrip,
		testLegacyCreateAccountRoundTrip,
		testOpenAccountsRoundTrip,
		testSendPrivatePaymentRoundTrip,
		testReceivePaymentsPrivatelyRoundTrip,
		testSaveRecentRootRoundTrip,
		testMigrateToPrivacy2022RoundTrip,
		testExternalDepositRoundTrip,
		testSendPublicPaymentRoundTrip,
		testReceivePaymentsPubliclyRoundTrip,
		testEstablishRelationshipRoundTrip,
		testLoginRoundTrip,
		testUpdate,
		testGetLatestByInitiatorAndType,
		testGetCountForAntispam,
		testGetOwnerInteractionCountForAntispam,
		testGetTransactedAmountForAntiMoneyLaundering,
		testGetDepositedAmountForAntiMoneyLaundering,
		testGetNetBalanceFromPrePrivacyIntents,
		testGetLatestSaveRecentRootIntentForTreasury,
		testGetOriginalGiftCardIssuedIntent,
		testGetGiftCardClaimedIntent,
	} {
		tf(t, s)
		teardown()
	}
}

func testLegacyPaymentRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testLegacyPaymentRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.LegacyPayment,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			MoneyTransferMetadata: &intent.MoneyTransferMetadata{
				Source:           "test_source",
				Destination:      "test_destination",
				Quantity:         42,
				ExchangeCurrency: currency.CAD,
				ExchangeRate:     0.00073,
				UsdMarketValue:   0.00042,
				IsWithdrawal:     true,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.MoneyTransferMetadata)
		assert.Equal(t, cloned.MoneyTransferMetadata.Source, actual.MoneyTransferMetadata.Source)
		assert.Equal(t, cloned.MoneyTransferMetadata.Destination, actual.MoneyTransferMetadata.Destination)
		assert.Equal(t, cloned.MoneyTransferMetadata.Quantity, actual.MoneyTransferMetadata.Quantity)
		assert.Equal(t, cloned.MoneyTransferMetadata.ExchangeCurrency, actual.MoneyTransferMetadata.ExchangeCurrency)
		assert.Equal(t, cloned.MoneyTransferMetadata.ExchangeRate, actual.MoneyTransferMetadata.ExchangeRate)
		assert.Equal(t, cloned.MoneyTransferMetadata.UsdMarketValue, actual.MoneyTransferMetadata.UsdMarketValue)
		assert.Equal(t, cloned.MoneyTransferMetadata.IsWithdrawal, actual.MoneyTransferMetadata.IsWithdrawal)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testLegacyCreateAccountRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testLegacyCreateAccountRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.LegacyCreateAccount,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			AccountManagementMetadata: &intent.AccountManagementMetadata{
				TokenAccount: "test_account",
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.AccountManagementMetadata)
		assert.Equal(t, cloned.AccountManagementMetadata.TokenAccount, actual.AccountManagementMetadata.TokenAccount)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testOpenAccountsRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testOpenAccountsRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.OpenAccounts,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			OpenAccountsMetadata:  &intent.OpenAccountsMetadata{},
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
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.OpenAccountsMetadata)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testSendPrivatePaymentRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testSendPrivatePaymentRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.SendPrivatePayment,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
				DestinationOwnerAccount: "test_destination_owner",
				DestinationTokenAccount: "test_destination_token",
				Quantity:                12345,

				ExchangeCurrency: currency.CAD,
				ExchangeRate:     0.00073,
				NativeAmount:     0.00073 * 12345,
				UsdMarketValue:   0.00042,
				IsWithdrawal:     true,
				IsRemoteSend:     true,
				IsMicroPayment:   true,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.SendPrivatePaymentMetadata)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.DestinationOwnerAccount, actual.SendPrivatePaymentMetadata.DestinationOwnerAccount)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.DestinationTokenAccount, actual.SendPrivatePaymentMetadata.DestinationTokenAccount)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.Quantity, actual.SendPrivatePaymentMetadata.Quantity)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.ExchangeCurrency, actual.SendPrivatePaymentMetadata.ExchangeCurrency)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.ExchangeRate, actual.SendPrivatePaymentMetadata.ExchangeRate)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.NativeAmount, actual.SendPrivatePaymentMetadata.NativeAmount)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.UsdMarketValue, actual.SendPrivatePaymentMetadata.UsdMarketValue)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.IsWithdrawal, actual.SendPrivatePaymentMetadata.IsWithdrawal)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.IsRemoteSend, actual.SendPrivatePaymentMetadata.IsRemoteSend)
		assert.Equal(t, cloned.SendPrivatePaymentMetadata.IsMicroPayment, actual.SendPrivatePaymentMetadata.IsMicroPayment)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testReceivePaymentsPrivatelyRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testReceivePaymentsPrivatelyRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.ReceivePaymentsPrivately,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{
				Source:         "test_source",
				Quantity:       12345,
				IsDeposit:      true,
				UsdMarketValue: 777,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.ReceivePaymentsPrivatelyMetadata)
		assert.Equal(t, cloned.ReceivePaymentsPrivatelyMetadata.Source, actual.ReceivePaymentsPrivatelyMetadata.Source)
		assert.Equal(t, cloned.ReceivePaymentsPrivatelyMetadata.Quantity, actual.ReceivePaymentsPrivatelyMetadata.Quantity)
		assert.Equal(t, cloned.ReceivePaymentsPrivatelyMetadata.IsDeposit, actual.ReceivePaymentsPrivatelyMetadata.IsDeposit)
		assert.Equal(t, cloned.ReceivePaymentsPrivatelyMetadata.UsdMarketValue, actual.ReceivePaymentsPrivatelyMetadata.UsdMarketValue)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testSaveRecentRootRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testSaveRecentRootRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.SaveRecentRoot,
			InitiatorOwnerAccount: "test_owner",
			SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{
				TreasuryPool:           "test_treasury_pool",
				PreviousMostRecentRoot: "test_recent_root",
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Nil(t, actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.SaveRecentRootMetadata)
		assert.Equal(t, cloned.SaveRecentRootMetadata.TreasuryPool, actual.SaveRecentRootMetadata.TreasuryPool)
		assert.Equal(t, cloned.SaveRecentRootMetadata.PreviousMostRecentRoot, actual.SaveRecentRootMetadata.PreviousMostRecentRoot)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testMigrateToPrivacy2022RoundTrip(t *testing.T, s intent.Store) {
	t.Run("testMigrateToPrivacy2022RoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.MigrateToPrivacy2022,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			MigrateToPrivacy2022Metadata: &intent.MigrateToPrivacy2022Metadata{
				Quantity: 123,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.MigrateToPrivacy2022Metadata)
		assert.Equal(t, cloned.MigrateToPrivacy2022Metadata.Quantity, actual.MigrateToPrivacy2022Metadata.Quantity)
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
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Nil(t, actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.ExternalDepositMetadata)
		assert.Equal(t, cloned.ExternalDepositMetadata.DestinationOwnerAccount, actual.ExternalDepositMetadata.DestinationOwnerAccount)
		assert.Equal(t, cloned.ExternalDepositMetadata.DestinationTokenAccount, actual.ExternalDepositMetadata.DestinationTokenAccount)
		assert.Equal(t, cloned.ExternalDepositMetadata.Quantity, actual.ExternalDepositMetadata.Quantity)
		assert.Equal(t, cloned.ExternalDepositMetadata.UsdMarketValue, actual.ExternalDepositMetadata.UsdMarketValue)
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

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.SendPublicPayment,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
				DestinationOwnerAccount: "test_destination_owner",
				DestinationTokenAccount: "test_destination_token",
				Quantity:                12345,

				ExchangeCurrency: currency.CAD,
				ExchangeRate:     0.00073,
				NativeAmount:     0.00073 * 12345,
				UsdMarketValue:   0.00042,
				IsWithdrawal:     true,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.SendPublicPaymentMetadata)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.DestinationOwnerAccount, actual.SendPublicPaymentMetadata.DestinationOwnerAccount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.DestinationTokenAccount, actual.SendPublicPaymentMetadata.DestinationTokenAccount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.Quantity, actual.SendPublicPaymentMetadata.Quantity)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.ExchangeCurrency, actual.SendPublicPaymentMetadata.ExchangeCurrency)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.ExchangeRate, actual.SendPublicPaymentMetadata.ExchangeRate)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.NativeAmount, actual.SendPublicPaymentMetadata.NativeAmount)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.UsdMarketValue, actual.SendPublicPaymentMetadata.UsdMarketValue)
		assert.Equal(t, cloned.SendPublicPaymentMetadata.IsWithdrawal, actual.SendPublicPaymentMetadata.IsWithdrawal)
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

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.ReceivePaymentsPublicly,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
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
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
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
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testEstablishRelationshipRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testEstablishRelationshipRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.EstablishRelationship,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			EstablishRelationshipMetadata: &intent.EstablishRelationshipMetadata{
				RelationshipTo: "relationship_to",
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.EstablishRelationshipMetadata)
		assert.Equal(t, cloned.EstablishRelationshipMetadata.RelationshipTo, actual.EstablishRelationshipMetadata.RelationshipTo)
		assert.Equal(t, cloned.State, actual.State)
		assert.Equal(t, cloned.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assert.EqualValues(t, 1, actual.Id)
	})
}

func testLoginRoundTrip(t *testing.T, s intent.Store) {
	t.Run("testLoginRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.Get(ctx, "test_intent_id")
		require.Error(t, err)
		assert.Equal(t, intent.ErrIntentNotFound, err)
		assert.Nil(t, actual)

		phoneNumberValue := "+12223334444"
		expected := intent.Record{
			IntentId:              "test_intent_id",
			IntentType:            intent.Login,
			InitiatorOwnerAccount: "test_owner",
			InitiatorPhoneNumber:  &phoneNumberValue,
			LoginMetadata: &intent.LoginMetadata{
				App:    "app",
				UserId: "test_user",
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, &expected)
		require.NoError(t, err)

		actual, err = s.Get(ctx, "test_intent_id")
		require.NoError(t, err)
		assert.Equal(t, cloned.IntentId, actual.IntentId)
		assert.Equal(t, cloned.IntentType, actual.IntentType)
		assert.Equal(t, cloned.InitiatorOwnerAccount, actual.InitiatorOwnerAccount)
		assert.Equal(t, *cloned.InitiatorPhoneNumber, *actual.InitiatorPhoneNumber)
		require.NotNil(t, actual.LoginMetadata)
		assert.Equal(t, cloned.LoginMetadata.App, actual.LoginMetadata.App)
		assert.Equal(t, cloned.LoginMetadata.UserId, actual.LoginMetadata.UserId)
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
			IntentType:            intent.LegacyPayment,
			InitiatorOwnerAccount: "test_owner",
			MoneyTransferMetadata: &intent.MoneyTransferMetadata{
				Source:           "test_source",
				Destination:      "test_destination",
				Quantity:         42,
				ExchangeCurrency: currency.CAD,
				ExchangeRate:     0.00073,
				UsdMarketValue:   0.00042,
			},
			State:     intent.StateUnknown,
			CreatedAt: time.Now(),
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
			{IntentId: "t1", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o1", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a3", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StatePending},
			{IntentId: "t2", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o1", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a4", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StateFailed},
			{IntentId: "t3", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o1", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a5", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StateUnknown},
			{IntentId: "t4", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o1", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a6", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StateUnknown},
			{IntentId: "t5", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o2", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a2", Destination: "a7", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StateUnknown},
			{IntentId: "t6", IntentType: intent.LegacyPayment, InitiatorOwnerAccount: "o2", MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a2", Destination: "a8", Quantity: 42, ExchangeCurrency: currency.CAD, ExchangeRate: 0.0007, UsdMarketValue: 0.01}, State: intent.StateFailed},
		}
		for i, record := range records {
			record.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
			require.NoError(t, s.Save(ctx, &record))
		}

		_, err := s.GetLatestByInitiatorAndType(ctx, intent.LegacyCreateAccount, "o1")
		assert.Equal(t, intent.ErrIntentNotFound, err)

		actual, err := s.GetLatestByInitiatorAndType(ctx, intent.LegacyPayment, "o1")
		require.NoError(t, err)
		assert.Equal(t, "t4", actual.IntentId)
	})

}

func testGetCountForAntispam(t *testing.T, s intent.Store) {
	t.Run("testGetCountForAntispam", func(t *testing.T) {
		ctx := context.Background()

		phoneNumber := "+12223334444"
		otherPhoneNumber := "+18005550000"

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.LegacyCreateAccount, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, AccountManagementMetadata: &intent.AccountManagementMetadata{TokenAccount: "t1"}, State: intent.StateUnknown, CreatedAt: time.Now().Add(-1 * time.Minute)},
			{IntentId: "i2", IntentType: intent.LegacyCreateAccount, InitiatorOwnerAccount: "o2", InitiatorPhoneNumber: &phoneNumber, AccountManagementMetadata: &intent.AccountManagementMetadata{TokenAccount: "t2"}, State: intent.StatePending, CreatedAt: time.Now().Add(-2 * time.Minute)},
			{IntentId: "i3", IntentType: intent.LegacyCreateAccount, InitiatorOwnerAccount: "o3", InitiatorPhoneNumber: &phoneNumber, AccountManagementMetadata: &intent.AccountManagementMetadata{TokenAccount: "t3"}, State: intent.StateConfirmed, CreatedAt: time.Now().Add(-3 * time.Minute)},
			{IntentId: "i4", IntentType: intent.LegacyCreateAccount, InitiatorOwnerAccount: "o4", InitiatorPhoneNumber: &phoneNumber, AccountManagementMetadata: &intent.AccountManagementMetadata{TokenAccount: "t4"}, State: intent.StateFailed, CreatedAt: time.Now().Add(-4 * time.Minute)},
			{IntentId: "i5", IntentType: intent.LegacyCreateAccount, InitiatorOwnerAccount: "o5", InitiatorPhoneNumber: &phoneNumber, AccountManagementMetadata: &intent.AccountManagementMetadata{TokenAccount: "t5"}, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		allStates := []intent.State{
			intent.StateUnknown,
			intent.StatePending,
			intent.StateConfirmed,
			intent.StateFailed,
			intent.StateRevoked,
		}

		// Capture all intents for the phone number
		count, err := s.CountForAntispam(ctx, intent.LegacyCreateAccount, phoneNumber, allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 5, count)

		// Capture a subset of intents based on time
		count, err = s.CountForAntispam(ctx, intent.LegacyCreateAccount, phoneNumber, allStates, time.Now().Add(-90*time.Second))
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		// Capture a subset of intents based on state
		count, err = s.CountForAntispam(ctx, intent.LegacyCreateAccount, phoneNumber, []intent.State{intent.StateUnknown, intent.StatePending}, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)

		// Capture no intents because the type mismatches
		count, err = s.CountForAntispam(ctx, intent.LegacyPayment, phoneNumber, allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Capture no intents because the phone number mismatches
		count, err = s.CountForAntispam(ctx, intent.LegacyCreateAccount, otherPhoneNumber, allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func testGetOwnerInteractionCountForAntispam(t *testing.T, s intent.Store) {
	t.Run("testGetOwnerInteractionCountForAntispam", func(t *testing.T) {
		ctx := context.Background()

		phoneNumber := "+12223334444"
		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "t2", Quantity: 1, ExchangeCurrency: currency.KIN, ExchangeRate: 1.0, NativeAmount: 1.0, UsdMarketValue: 1.0}, State: intent.StateUnknown, CreatedAt: time.Now().Add(-1 * time.Minute)},
			{IntentId: "i2", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "t2", Quantity: 1, ExchangeCurrency: currency.KIN, ExchangeRate: 1.0, NativeAmount: 1.0, UsdMarketValue: 1.0}, State: intent.StatePending, CreatedAt: time.Now().Add(-2 * time.Minute)},
			{IntentId: "i3", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "t2", Quantity: 1, ExchangeCurrency: currency.KIN, ExchangeRate: 1.0, NativeAmount: 1.0, UsdMarketValue: 1.0}, State: intent.StateConfirmed, CreatedAt: time.Now().Add(-3 * time.Minute)},
			{IntentId: "i4", IntentType: intent.SendPublicPayment, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "t2", Quantity: 1, ExchangeCurrency: currency.KIN, ExchangeRate: 1.0, NativeAmount: 1.0, UsdMarketValue: 1.0}, State: intent.StateFailed, CreatedAt: time.Now().Add(-4 * time.Minute)},
			{IntentId: "i5", IntentType: intent.SendPublicPayment, InitiatorOwnerAccount: "o1", InitiatorPhoneNumber: &phoneNumber, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "t2", Quantity: 1, ExchangeCurrency: currency.KIN, ExchangeRate: 1.0, NativeAmount: 1.0, UsdMarketValue: 1.0}, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		allStates := []intent.State{
			intent.StateUnknown,
			intent.StatePending,
			intent.StateConfirmed,
			intent.StateFailed,
			intent.StateRevoked,
		}

		// Capture all intents for the owner pair
		count, err := s.CountOwnerInteractionsForAntispam(ctx, "o1", "o2", allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 5, count)

		// Capture no intents for incorrect owner pairs

		count, err = s.CountOwnerInteractionsForAntispam(ctx, "o2", "o1", allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountOwnerInteractionsForAntispam(ctx, "o1", "o1", allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		count, err = s.CountOwnerInteractionsForAntispam(ctx, "o2", "o2", allStates, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Capture a subset of intents for the owner pair based on time
		count, err = s.CountOwnerInteractionsForAntispam(ctx, "o1", "o2", allStates, time.Now().Add(-90*time.Second))
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)

		// Capture a subset of intents for the owner pair based on state
		count, err = s.CountOwnerInteractionsForAntispam(ctx, "o1", "o2", []intent.State{intent.StateUnknown, intent.StatePending}, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 2, count)
	})
}

func testGetTransactedAmountForAntiMoneyLaundering(t *testing.T, s intent.Store) {
	t.Run("testGetTransactedAmountForAntiMoneyLaundering", func(t *testing.T) {
		ctx := context.Background()

		phoneNumber := "+12223334444"

		// No intents results in zero transacted values
		quarks, usdMarketValue, err := s.GetTransactedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, quarks)
		assert.EqualValues(t, 0, usdMarketValue)

		records := []intent.Record{
			{IntentId: "t1", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o1", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o1", DestinationTokenAccount: "a1", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 2, UsdMarketValue: 2}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateUnknown, CreatedAt: time.Now().Add(-1 * time.Minute)},
			{IntentId: "t2", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o2", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o2", DestinationTokenAccount: "a2", Quantity: 10, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 20, UsdMarketValue: 20}, InitiatorPhoneNumber: &phoneNumber, State: intent.StatePending, CreatedAt: time.Now().Add(-2 * time.Minute)},
			{IntentId: "t3", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o3", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o3", DestinationTokenAccount: "a3", Quantity: 100, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 200, UsdMarketValue: 200}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateConfirmed, CreatedAt: time.Now().Add(-3 * time.Minute)},
			{IntentId: "t4", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o4", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o4", DestinationTokenAccount: "a4", Quantity: 1000, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 2000, UsdMarketValue: 2000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateFailed, CreatedAt: time.Now().Add(-4 * time.Minute)},
			{IntentId: "t5", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o5", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o5", DestinationTokenAccount: "a5", Quantity: 10000, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 20000, UsdMarketValue: 20000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
			{IntentId: "t6", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o6", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "a6", Quantity: 100000, UsdMarketValue: 200000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateConfirmed, CreatedAt: time.Now()},
			{IntentId: "t7", IntentType: intent.SendPublicPayment, InitiatorOwnerAccount: "o7", SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationOwnerAccount: "o7", DestinationTokenAccount: "a7", Quantity: 1000000, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 2000000, UsdMarketValue: 20000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateConfirmed, CreatedAt: time.Now()},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		// Capture all intents for the phone number
		quarks, usdMarketValue, err = s.GetTransactedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 1111, quarks)
		assert.EqualValues(t, 2222, usdMarketValue)

		// Capture a subset of intents based on time
		quarks, usdMarketValue, err = s.GetTransactedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-150*time.Second))
		require.NoError(t, err)
		assert.EqualValues(t, 11, quarks)
		assert.EqualValues(t, 22, usdMarketValue)

		// Capture no intents because the phone number mismatches
		quarks, usdMarketValue, err = s.GetTransactedAmountForAntiMoneyLaundering(ctx, "+18005550000", time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, quarks)
		assert.EqualValues(t, 0, usdMarketValue)
	})
}

func testGetDepositedAmountForAntiMoneyLaundering(t *testing.T, s intent.Store) {
	t.Run("testGetDepositedAmountForAntiMoneyLaundering", func(t *testing.T) {
		ctx := context.Background()

		phoneNumber := "+12223334444"

		// No intents results in zero transacted values
		quarks, usdMarketValue, err := s.GetTransactedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, quarks)
		assert.EqualValues(t, 0, usdMarketValue)

		records := []intent.Record{
			{IntentId: "t1", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o1", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: true, Source: "a1", Quantity: 1, UsdMarketValue: 2}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateUnknown, CreatedAt: time.Now().Add(-1 * time.Minute)},
			{IntentId: "t2", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o2", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: true, Source: "a2", Quantity: 10, UsdMarketValue: 20}, InitiatorPhoneNumber: &phoneNumber, State: intent.StatePending, CreatedAt: time.Now().Add(-2 * time.Minute)},
			{IntentId: "t3", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o3", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: true, Source: "a3", Quantity: 100, UsdMarketValue: 200}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateConfirmed, CreatedAt: time.Now().Add(-3 * time.Minute)},
			{IntentId: "t4", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o4", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: true, Source: "a4", Quantity: 1000, UsdMarketValue: 2000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateFailed, CreatedAt: time.Now().Add(-4 * time.Minute)},
			{IntentId: "t5", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o5", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: true, Source: "a5", Quantity: 10000, UsdMarketValue: 20000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
			{IntentId: "t6", IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: "o6", ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{IsDeposit: false, Source: "a6", Quantity: 100000, UsdMarketValue: 200000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
			{IntentId: "t7", IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: "o7", SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationOwnerAccount: "o7", DestinationTokenAccount: "a7", Quantity: 1000000, ExchangeCurrency: currency.USD, ExchangeRate: 2, NativeAmount: 2000000, UsdMarketValue: 2000000}, InitiatorPhoneNumber: &phoneNumber, State: intent.StateRevoked, CreatedAt: time.Now().Add(-5 * time.Minute)},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		// Capture all intents for the phone number
		quarks, usdMarketValue, err = s.GetDepositedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 1111, quarks)
		assert.EqualValues(t, 2222, usdMarketValue)

		// Capture a subset of intents based on time
		quarks, usdMarketValue, err = s.GetDepositedAmountForAntiMoneyLaundering(ctx, phoneNumber, time.Now().Add(-150*time.Second))
		require.NoError(t, err)
		assert.EqualValues(t, 11, quarks)
		assert.EqualValues(t, 22, usdMarketValue)

		// Capture no intents because the phone number mismatches
		quarks, usdMarketValue, err = s.GetDepositedAmountForAntiMoneyLaundering(ctx, "+18005550000", time.Now().Add(-24*time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, quarks)
		assert.EqualValues(t, 0, usdMarketValue)
	})
}

func testGetNetBalanceFromPrePrivacyIntents(t *testing.T, s intent.Store) {
	t.Run("testGetNetBalanceFromPrePrivacyIntents", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "o1", State: intent.StateUnknown},
			{IntentId: "i2", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a2", Quantity: 10, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 10}, InitiatorOwnerAccount: "o1", State: intent.StatePending},
			{IntentId: "i3", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a2", Quantity: 100, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 100}, InitiatorOwnerAccount: "o1", State: intent.StateConfirmed},
			{IntentId: "i4", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a2", Quantity: 1000, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 1000}, InitiatorOwnerAccount: "o1", State: intent.StateFailed},
			{IntentId: "i5", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a1", Destination: "a2", Quantity: 10000, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 10000}, InitiatorOwnerAccount: "o1", State: intent.StateRevoked},
			{IntentId: "i6", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 100000, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 100000, UsdMarketValue: 100000}, InitiatorOwnerAccount: "o1", State: intent.StateConfirmed},
			{IntentId: "i7", IntentType: intent.ReceivePaymentsPrivately, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "a2", Quantity: 100000, UsdMarketValue: 100000}, InitiatorOwnerAccount: "o1", State: intent.StateConfirmed},
			{IntentId: "i8", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a3", Destination: "a3", Quantity: 100000, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 100000}, InitiatorOwnerAccount: "o3", State: intent.StateConfirmed},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		netBalance, err := s.GetNetBalanceFromPrePrivacy2022Intents(ctx, "a1")
		require.NoError(t, err)
		assert.EqualValues(t, -1110, netBalance)

		netBalance, err = s.GetNetBalanceFromPrePrivacy2022Intents(ctx, "a2")
		require.NoError(t, err)
		assert.EqualValues(t, 1110, netBalance)

		netBalance, err = s.GetNetBalanceFromPrePrivacy2022Intents(ctx, "a3")
		require.NoError(t, err)
		assert.EqualValues(t, 0, netBalance)

		netBalance, err = s.GetNetBalanceFromPrePrivacy2022Intents(ctx, "a4")
		require.NoError(t, err)
		assert.EqualValues(t, 0, netBalance)
	})
}

func testGetLatestSaveRecentRootIntentForTreasury(t *testing.T, s intent.Store) {
	t.Run("testGetLatestSaveRecentRootIntentForTreasury", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.SaveRecentRoot, SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{TreasuryPool: "t1", PreviousMostRecentRoot: "rr1"}, InitiatorOwnerAccount: "code", State: intent.StateConfirmed},
			{IntentId: "i2", IntentType: intent.SaveRecentRoot, SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{TreasuryPool: "t1", PreviousMostRecentRoot: "rr2"}, InitiatorOwnerAccount: "code", State: intent.StateConfirmed},
			{IntentId: "i3", IntentType: intent.SaveRecentRoot, SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{TreasuryPool: "t1", PreviousMostRecentRoot: "rr3"}, InitiatorOwnerAccount: "code", State: intent.StatePending},
			{IntentId: "i4", IntentType: intent.SaveRecentRoot, SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{TreasuryPool: "t2", PreviousMostRecentRoot: "rr4"}, InitiatorOwnerAccount: "code", State: intent.StateConfirmed},
			{IntentId: "i5", IntentType: intent.SaveRecentRoot, SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{TreasuryPool: "t2", PreviousMostRecentRoot: "rr5"}, InitiatorOwnerAccount: "code", State: intent.StateConfirmed},
		}

		for _, record := range records {
			require.NoError(t, s.Save(ctx, &record))
		}

		intentRecord, err := s.GetLatestSaveRecentRootIntentForTreasury(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, "i3", intentRecord.IntentId)
		assert.Equal(t, "t1", intentRecord.SaveRecentRootMetadata.TreasuryPool)
		assert.Equal(t, "rr3", intentRecord.SaveRecentRootMetadata.PreviousMostRecentRoot)

		intentRecord, err = s.GetLatestSaveRecentRootIntentForTreasury(ctx, "t2")
		require.NoError(t, err)
		assert.Equal(t, "i5", intentRecord.IntentId)
		assert.Equal(t, "t2", intentRecord.SaveRecentRootMetadata.TreasuryPool)
		assert.Equal(t, "rr5", intentRecord.SaveRecentRootMetadata.PreviousMostRecentRoot)

		_, err = s.GetLatestSaveRecentRootIntentForTreasury(ctx, "t3")
		assert.Equal(t, intent.ErrIntentNotFound, err)
	})
}

func testGetOriginalGiftCardIssuedIntent(t *testing.T, s intent.Store) {
	t.Run("testGetOriginalGiftCardIssuedIntent", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: false, DestinationTokenAccount: "a1", DestinationOwnerAccount: "o1", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i2", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i3", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: false, DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i4", IntentType: intent.SendPublicPayment, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i5", IntentType: intent.ExternalDeposit, ExternalDepositMetadata: &intent.ExternalDepositMetadata{DestinationTokenAccount: "a2", DestinationOwnerAccount: "o2", Quantity: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i6", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "source", Destination: "a2", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i7", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a3", DestinationOwnerAccount: "o3", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i8", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a3", DestinationOwnerAccount: "o3", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i9", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a4", DestinationOwnerAccount: "o4", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StatePending},
			{IntentId: "i10", IntentType: intent.SendPrivatePayment, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{IsRemoteSend: true, DestinationTokenAccount: "a4", DestinationOwnerAccount: "o4", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, NativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateRevoked},
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
		assert.Equal(t, "i9", actual.IntentId)
	})
}

func testGetGiftCardClaimedIntent(t *testing.T, s intent.Store) {
	t.Run("testGetGiftCardClaimedIntent", func(t *testing.T) {
		ctx := context.Background()

		records := []intent.Record{
			{IntentId: "i1", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: false, Source: "a1", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i2", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: false, Source: "a2", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i3", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a2", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i4", IntentType: intent.ReceivePaymentsPrivately, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "a2", Quantity: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i5", IntentType: intent.LegacyPayment, MoneyTransferMetadata: &intent.MoneyTransferMetadata{Source: "a2", Destination: "destination", Quantity: 1, ExchangeCurrency: currency.USD, ExchangeRate: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i6", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a3", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},
			{IntentId: "i7", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a3", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateConfirmed},

			{IntentId: "i8", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a4", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StateRevoked},
			{IntentId: "i9", IntentType: intent.ReceivePaymentsPublicly, ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{IsRemoteSend: true, Source: "a4", Quantity: 1, OriginalExchangeCurrency: currency.USD, OriginalExchangeRate: 1, OriginalNativeAmount: 1, UsdMarketValue: 1}, InitiatorOwnerAccount: "user", State: intent.StatePending},
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
		assert.Equal(t, "i9", actual.IntentId)
	})
}
