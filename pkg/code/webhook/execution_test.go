package webhook

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestWebhook_HappyPath_IntentSubmmitted(t *testing.T) {
	env := setup(t)

	webhookRecord := env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	intentRecord := env.setupIntentRecord(t, webhookRecord)
	accountInfoRecord := env.setupRelationshipAccount(t, intentRecord.InitiatorOwnerAccount)

	require.NoError(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))

	requests := env.server.GetReceivedRequests()
	require.Len(t, requests, 1)

	parsed, err := jwt.ParseWithClaims(requests[0], jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		return ed25519.PublicKey(common.GetSubsidizer().PublicKey().ToBytes()), nil
	})
	require.NoError(t, err)

	claims := parsed.Claims.(jwt.MapClaims)
	require.Len(t, claims, 9)
	assert.Equal(t, intentRecord.IntentId, claims["intent"])
	assert.Equal(t, strings.ToUpper(string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency)), claims["currency"])
	assert.Equal(t, intentRecord.SendPrivatePaymentMetadata.NativeAmount, claims["amount"])
	assert.Equal(t, intentRecord.SendPrivatePaymentMetadata.ExchangeRate, claims["exchangeRate"])
	assert.EqualValues(t, intentRecord.SendPrivatePaymentMetadata.Quantity, claims["quarks"])
	assert.EqualValues(t, 12345, claims["fees"])
	assert.Equal(t, intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount, claims["destination"])
	assert.Equal(t, accountInfoRecord.AuthorityAccount, claims["user"])
	assert.Equal(t, "SUBMITTED", claims["state"])

	env.assertWebhookCalledMessageSent(t, webhookRecord)
}

func TestWebhook_EndpointError(t *testing.T) {
	env := setup(t)
	env.server.SimulateErrors()

	webhookRecord := env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	env.setupIntentRecord(t, webhookRecord)

	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))

	env.assertNoWebhookCalledMessagesSent(t, webhookRecord)
}

func TestWebhook_Timeout(t *testing.T) {
	env := setup(t)
	env.server.SimulateDelay(200 * time.Millisecond)

	webhookRecord := env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	env.setupIntentRecord(t, webhookRecord)

	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, 100*time.Millisecond))

	env.assertNoWebhookCalledMessagesSent(t, webhookRecord)
}

func TestWebhook_Validation_Generic(t *testing.T) {
	env := setup(t)

	webhookRecord := env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	env.setupIntentRecord(t, webhookRecord)
	for _, invalidWebhookState := range []webhook.State{
		webhook.StateUnknown,
		webhook.StateFailed,
		webhook.StateConfirmed,
	} {
		webhookRecord.State = invalidWebhookState
		assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))
		assert.Empty(t, env.server.GetReceivedRequests())
	}

	webhookRecord = env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	env.setupIntentRecord(t, webhookRecord)
	webhookRecord.NextAttemptAt = pointer.Time(time.Now().Add(time.Second))
	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))
	assert.Empty(t, env.server.GetReceivedRequests())

	webhookRecord = env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	webhookRecord.WebhookId = "not-a-public-key"
	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))
	assert.Empty(t, env.server.GetReceivedRequests())
}

func TestWebhook_Validation_IntentSubmitted(t *testing.T) {
	env := setup(t)

	webhookRecord := env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))
	assert.Empty(t, env.server.GetReceivedRequests())

	webhookRecord = env.server.GetRandomWebhookRecord(t, webhook.TypeIntentSubmitted)
	intentRecord := env.setupIntentRecord(t, webhookRecord)
	intentRecord.State = intent.StateRevoked
	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
	assert.Error(t, Execute(env.ctx, env.data, env.messagingClient, webhookRecord, time.Second))
	assert.Empty(t, env.server.GetReceivedRequests())
}

type testEnv struct {
	ctx             context.Context
	data            code_data.Provider
	messagingClient messaging.InternalMessageClient
	server          *TestWebhookEndpoint
}

func setup(t *testing.T) testEnv {
	data := code_data.NewTestDataProvider()
	testutil.SetupRandomSubsidizer(t, data)
	return testEnv{
		ctx:             context.Background(),
		data:            data,
		messagingClient: messaging.NewMessagingClient(data),
		server:          NewTestWebhookEndpoint(t),
	}
}

func (e *testEnv) setupIntentRecord(t *testing.T, webhookRecord *webhook.Record) *intent.Record {
	require.Equal(t, webhook.TypeIntentSubmitted, webhookRecord.Type)

	intentRecord := &intent.Record{
		IntentId:   webhookRecord.WebhookId,
		IntentType: intent.SendPrivatePayment,
		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			ExchangeCurrency:        "usd",
			NativeAmount:            12.3,
			ExchangeRate:            0.1,
			Quantity:                kin.ToQuarks(123),
			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IsMicroPayment:          true,
			UsdMarketValue:          12.3,
		},
		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	fees := uint64(12345)
	thirdPartyPaymentAmount := intentRecord.SendPrivatePaymentMetadata.Quantity - fees
	actionsRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   8,
		ActionType: action.NoPrivacyWithdraw,

		Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,

		Quantity: &thirdPartyPaymentAmount,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, actionsRecord))

	paymentRequestRecord := &paymentrequest.Record{
		Intent: intentRecord.IntentId,

		DestinationTokenAccount: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
		ExchangeCurrency:        pointer.String(string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency)),
		NativeAmount:            &intentRecord.SendPrivatePaymentMetadata.NativeAmount,

		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}
	require.NoError(t, e.data.CreateRequest(e.ctx, paymentRequestRecord))

	return intentRecord
}

func (e *testEnv) setupRelationshipAccount(t *testing.T, owner string) *account.Record {
	accountInfoRecord := &account.Record{
		OwnerAccount:     owner,
		AuthorityAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		Index:          0,
		RelationshipTo: pointer.String("example.com"),

		CreatedAt: time.Now(),
	}
	require.NoError(t, e.data.CreateAccountInfo(e.ctx, accountInfoRecord))
	return accountInfoRecord
}

func (e *testEnv) assertWebhookCalledMessageSent(t *testing.T, webhookRecord *webhook.Record) {
	messageRecords, err := e.data.GetMessages(e.ctx, webhookRecord.WebhookId)
	require.NoError(t, err)
	require.Len(t, messageRecords, 1)

	var protoMessage messagingpb.Message
	require.NoError(t, proto.Unmarshal(messageRecords[0].Message, &protoMessage))
	require.NotNil(t, protoMessage.GetWebhookCalled())
}

func (e *testEnv) assertNoWebhookCalledMessagesSent(t *testing.T, webhookRecord *webhook.Record) {
	messageRecords, err := e.data.GetMessages(e.ctx, webhookRecord.WebhookId)
	require.NoError(t, err)
	assert.Empty(t, messageRecords)
}
