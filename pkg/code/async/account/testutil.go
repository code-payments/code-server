package async_account

/*
import (
	"context"
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
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx     context.Context
	data    code_data.Provider
	service *service
}

type testGiftCard struct {
	accountInfoRecord *account.Record

	issuedIntentRecord            *intent.Record
	claimedActionRecord           *action.Record
	closeDormantActionRecord      *action.Record
	closeDormantFulfillmentRecord *fulfillment.Record
}

func setup(t *testing.T) *testEnv {
	data := code_data.NewTestDataProvider()

	require.NoError(t, common.InjectTestSubsidizer(context.Background(), data, testutil.NewRandomAccount(t)))

	require.NoError(t, data.ImportExchangeRates(context.Background(), &currency.MultiRateRecord{
		Time: time.Now(),
		Rates: map[string]float64{
			"usd": 0.1,
		},
	}))

	return &testEnv{
		ctx:     context.Background(),
		data:    data,
		service: New(data, WithEnvConfigs()).(*service),
	}
}

func (e *testEnv) generateRandomGiftCard(t *testing.T, creationTs time.Time) *testGiftCard {
	vm := testutil.NewRandomAccount(t)
	authority := testutil.NewRandomAccount(t)

	timelockAccounts, err := authority.GetTimelockAccounts(vm, common.CoreMintAccount)
	require.NoError(t, err)

	accountInfoRecord := &account.Record{
		OwnerAccount:     authority.PublicKey().ToBase58(),
		AuthorityAccount: authority.PublicKey().ToBase58(),
		TokenAccount:     timelockAccounts.Vault.PublicKey().ToBase58(),
		MintAccount:      timelockAccounts.Mint.PublicKey().ToBase58(),

		AccountType: commonpb.AccountType_REMOTE_SEND_GIFT_CARD,

		RequiresAutoReturnCheck: true,

		CreatedAt: creationTs,
	}
	require.NoError(t, e.data.CreateAccountInfo(e.ctx, accountInfoRecord))

	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.SendPrivatePayment,

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			DestinationTokenAccount: accountInfoRecord.TokenAccount,
			Quantity:                common.ToCoreMintQuarks(12345),

			ExchangeCurrency: common.CoreMintSymbol,
			ExchangeRate:     1.0,
			NativeAmount:     12345,
			UsdMarketValue:   1000.0,

			IsRemoteSend: true,
		},

		State: intent.StatePending,

		CreatedAt: creationTs,
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	closeDormantActionRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   10,
		ActionType: action.CloseDormantAccount,

		Source:      accountInfoRecord.TokenAccount,
		Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		Quantity:    nil,

		State: action.StateUnknown,

		CreatedAt: creationTs,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, closeDormantActionRecord))

	closeDormantFulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   closeDormantActionRecord.ActionId,
		ActionType: closeDormantActionRecord.ActionType,

		FulfillmentType: fulfillment.CloseDormantTimelockAccount,
		Data:            []byte("data"),
		Signature:       pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),

		Nonce:     pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		Blockhash: pointer.String("bh"),

		Source:      closeDormantActionRecord.Source,
		Destination: pointer.StringCopy(closeDormantActionRecord.Destination),

		IntentOrderingIndex:      math.MaxInt64,
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		DisableActiveScheduling: true,

		State: fulfillment.StateUnknown,

		CreatedAt: creationTs,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, closeDormantFulfillmentRecord))

	return &testGiftCard{
		accountInfoRecord: accountInfoRecord,

		issuedIntentRecord:            intentRecord,
		closeDormantActionRecord:      closeDormantActionRecord,
		closeDormantFulfillmentRecord: closeDormantFulfillmentRecord,
	}
}

func (e *testEnv) simulateGiftCardBeingClaimed(t *testing.T, giftCard *testGiftCard) {
	require.Nil(t, giftCard.claimedActionRecord)

	giftCard.claimedActionRecord = &action.Record{
		Intent:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.ReceivePaymentsPublicly,

		ActionId:   0,
		ActionType: action.NoPrivacyWithdraw,

		Source:      giftCard.accountInfoRecord.TokenAccount,
		Destination: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		Quantity:    pointer.Uint64(giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.Quantity),

		State: action.StatePending,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, giftCard.claimedActionRecord))
}

func (e *testEnv) assertGiftCardAutoReturned(t *testing.T, giftCard *testGiftCard) {
	accountInfoRecord, err := e.data.GetAccountInfoByTokenAddress(e.ctx, giftCard.accountInfoRecord.TokenAccount)
	require.NoError(t, err)
	assert.False(t, accountInfoRecord.RequiresAutoReturnCheck)

	actionRecord, err := e.data.GetActionById(e.ctx, giftCard.closeDormantActionRecord.Intent, giftCard.closeDormantActionRecord.ActionId)
	require.NoError(t, err)
	require.NotNil(t, actionRecord.Quantity)
	assert.Equal(t, giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.Quantity, *actionRecord.Quantity)
	assert.Equal(t, action.StatePending, actionRecord.State)

	fulfillmentRecord, err := e.data.GetFulfillmentBySignature(e.ctx, *giftCard.closeDormantFulfillmentRecord.Signature)
	require.NoError(t, err)
	assert.EqualValues(t, giftCard.issuedIntentRecord.Id, fulfillmentRecord.IntentOrderingIndex)
	assert.EqualValues(t, math.MaxInt32, fulfillmentRecord.ActionOrderingIndex)
	assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
	assert.False(t, fulfillmentRecord.DisableActiveScheduling)
	assert.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)

	intentId := getAutoReturnIntentId(giftCard.issuedIntentRecord.IntentId)
	historyRecord, err := e.data.GetIntent(e.ctx, intentId)
	require.NoError(t, err)
	assert.Equal(t, intentId, historyRecord.IntentId)
	assert.Equal(t, intent.ReceivePaymentsPublicly, historyRecord.IntentType)
	assert.Equal(t, giftCard.issuedIntentRecord.InitiatorOwnerAccount, historyRecord.InitiatorOwnerAccount)
	require.NotNil(t, historyRecord.ReceivePaymentsPubliclyMetadata)
	assert.Equal(t, giftCard.accountInfoRecord.TokenAccount, historyRecord.ReceivePaymentsPubliclyMetadata.Source)
	assert.Equal(t, giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.Quantity, historyRecord.ReceivePaymentsPubliclyMetadata.Quantity)
	assert.True(t, historyRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend)
	assert.True(t, historyRecord.ReceivePaymentsPubliclyMetadata.IsReturned)
	assert.Equal(t, giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.ExchangeCurrency, historyRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency)
	assert.Equal(t, giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.ExchangeRate, historyRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate)
	assert.Equal(t, giftCard.issuedIntentRecord.SendPrivatePaymentMetadata.NativeAmount, historyRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount)
	assert.Equal(t, 1234.5, historyRecord.ReceivePaymentsPubliclyMetadata.UsdMarketValue)
	assert.Equal(t, intent.StateConfirmed, historyRecord.State)
}

func (e *testEnv) assertGiftCardNotAutoReturned(t *testing.T, giftCard *testGiftCard, isRemovedFromWorkerQueue bool) {
	accountInfoRecord, err := e.data.GetAccountInfoByTokenAddress(e.ctx, giftCard.accountInfoRecord.TokenAccount)
	require.NoError(t, err)
	assert.Equal(t, isRemovedFromWorkerQueue, !accountInfoRecord.RequiresAutoReturnCheck)

	actionRecord, err := e.data.GetActionById(e.ctx, giftCard.closeDormantActionRecord.Intent, giftCard.closeDormantActionRecord.ActionId)
	require.NoError(t, err)
	assert.Nil(t, actionRecord.Quantity)
	assert.Equal(t, action.StateUnknown, actionRecord.State)

	fulfillmentRecord, err := e.data.GetFulfillmentBySignature(e.ctx, *giftCard.closeDormantFulfillmentRecord.Signature)
	require.NoError(t, err)
	assert.EqualValues(t, math.MaxInt64, fulfillmentRecord.IntentOrderingIndex)
	assert.EqualValues(t, 0, fulfillmentRecord.ActionOrderingIndex)
	assert.EqualValues(t, 0, fulfillmentRecord.FulfillmentOrderingIndex)
	assert.True(t, fulfillmentRecord.DisableActiveScheduling)
	assert.Equal(t, fulfillment.StateUnknown, fulfillmentRecord.State)

	_, err = e.data.GetIntent(e.ctx, giftCardAutoReturnIntentPrefix+giftCard.issuedIntentRecord.IntentId)
	assert.Equal(t, intent.ErrIntentNotFound, err)
}
*/
