package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestGetMicroPaymentReceiveExchangeDataByOwner(t *testing.T) {
	env := setup(t)

	micropaymentDestinationOwner := testutil.NewRandomAccount(t)
	additionalCodeUserDestinationOwner := testutil.NewRandomAccount(t)
	tempOutgoingAccount := testutil.NewRandomAccount(t)

	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.SendPrivatePayment,

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			DestinationOwnerAccount: micropaymentDestinationOwner.PublicKey().ToBase58(),
			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

			ExchangeCurrency: currency_lib.USD,
			ExchangeRate:     0.1,
			NativeAmount:     100,
			Quantity:         kin.ToQuarks(1000),
		},
	}

	actionRecords := []*action.Record{
		{
			ActionType:  action.NoPrivacyTransfer,
			Source:      tempOutgoingAccount.PublicKey().ToBase58(),
			Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
			Quantity:    pointer.Uint64(kin.ToQuarks(50)),
		},
		{
			ActionType:  action.NoPrivacyTransfer,
			Source:      tempOutgoingAccount.PublicKey().ToBase58(),
			Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
			Quantity:    pointer.Uint64(kin.ToQuarks(35)),
		},
		{
			ActionType:  action.NoPrivacyTransfer,
			Source:      tempOutgoingAccount.PublicKey().ToBase58(),
			Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
			Quantity:    pointer.Uint64(kin.ToQuarks(10)),
		},
		{
			ActionType:  action.NoPrivacyTransfer,
			Source:      tempOutgoingAccount.PublicKey().ToBase58(),
			Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
			Quantity:    pointer.Uint64(kin.ToQuarks(5)),
		},
		{
			ActionType:  action.NoPrivacyWithdraw,
			Source:      tempOutgoingAccount.PublicKey().ToBase58(),
			Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
			Quantity:    pointer.Uint64(kin.ToQuarks(900)),
		},
	}

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount,
		AuthorityAccount: intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount,
		TokenAccount:     *actionRecords[1].Destination,
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}))

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     additionalCodeUserDestinationOwner.PublicKey().ToBase58(),
		AuthorityAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		TokenAccount:     *actionRecords[2].Destination,
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_RELATIONSHIP,
		RelationshipTo:   pointer.String("example.com"),
	}))

	originalExchangeData, ok := getExchangeDataFromIntent(intentRecord)
	require.True(t, ok)

	exchangeDataByOwner, err := getMicroPaymentReceiveExchangeDataByOwner(env.ctx, env.data, originalExchangeData, intentRecord, actionRecords)
	require.NoError(t, err)
	require.Len(t, exchangeDataByOwner, 2)

	actualExchangeData, ok := exchangeDataByOwner[micropaymentDestinationOwner.PublicKey().ToBase58()]
	require.True(t, ok)
	assert.Equal(t, originalExchangeData.Currency, actualExchangeData.Currency)
	assert.Equal(t, originalExchangeData.ExchangeRate, actualExchangeData.ExchangeRate)
	assert.Equal(t, 93.5, actualExchangeData.NativeAmount)
	assert.Equal(t, kin.ToQuarks(935), actualExchangeData.Quarks)

	actualExchangeData, ok = exchangeDataByOwner[additionalCodeUserDestinationOwner.PublicKey().ToBase58()]
	require.True(t, ok)
	assert.Equal(t, originalExchangeData.Currency, actualExchangeData.Currency)
	assert.Equal(t, originalExchangeData.ExchangeRate, actualExchangeData.ExchangeRate)
	assert.Equal(t, 1.0, actualExchangeData.NativeAmount)
	assert.Equal(t, kin.ToQuarks(10), actualExchangeData.Quarks)
}
