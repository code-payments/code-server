package micropayment

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	micropaymentpb "github.com/code-payments/code-protobuf-api/generated/go/micropayment/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/messaging"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

var (
	baseInvalidUrlsToTest = []string{
		"ftp://download.me",
		"/just/a/path",
		"https://no-dns-tc/path",
	}
)

func TestGetStatus_Flags_HappyPath(t *testing.T) {
	paymentRequestRecord := &paymentrequest.Record{
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
	}
	loginRequestRecord := &paymentrequest.Record{
		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}

	paymentIntentRecord := &intent.Record{
		IntentType: intent.SendPrivatePayment,

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			ExchangeCurrency: currency_lib.Code(*paymentRequestRecord.ExchangeCurrency),
			NativeAmount:     *paymentRequestRecord.NativeAmount,
			ExchangeRate:     0.1,
			Quantity:         kin.ToQuarks(10),
			UsdMarketValue:   *paymentRequestRecord.NativeAmount,

			DestinationTokenAccount: *paymentRequestRecord.DestinationTokenAccount,

			IsWithdrawal: true,
		},

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		State: intent.StatePending,
	}

	loginIntentRecord := &intent.Record{
		IntentType: intent.Login,

		LoginMetadata: &intent.LoginMetadata{
			App:    "example.com",
			UserId: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		},

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

		State: intent.StateConfirmed,
	}

	for _, tc := range []struct {
		requestRecord *paymentrequest.Record
		intentRecord  *intent.Record
	}{
		{paymentRequestRecord, paymentIntentRecord},
		{loginRequestRecord, loginIntentRecord},
	} {
		env, cleanup := setup(t)
		defer cleanup()

		intentId := testutil.NewRandomAccount(t)

		req := &micropaymentpb.GetStatusRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
		}
		resp, err := env.client.GetStatus(env.ctx, req)
		require.NoError(t, err)
		assert.False(t, resp.Exists)
		assert.False(t, resp.CodeScanned)
		assert.False(t, resp.IntentSubmitted)

		tc.requestRecord.Intent = intentId.PublicKey().ToBase58()
		require.NoError(t, env.data.CreateRequest(env.ctx, tc.requestRecord))

		resp, err = env.client.GetStatus(env.ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Exists)
		assert.False(t, resp.CodeScanned)
		assert.False(t, resp.IntentSubmitted)

		messageId, err := uuid.NewRandom()
		require.NoError(t, err)
		codeScannedMessage := &messagingpb.Message{
			Id: &messagingpb.MessageId{
				Value: messageId[:],
			},
			Kind: &messagingpb.Message_CodeScanned{
				CodeScanned: &messagingpb.CodeScanned{
					Timestamp: timestamppb.Now(),
				},
			},
		}
		messageBytes, err := proto.Marshal(codeScannedMessage)
		require.NoError(t, err)
		messagingRecord := &messaging.Record{
			Account:   intentId.PublicKey().ToBase58(),
			MessageID: messageId,
			Message:   messageBytes,
		}
		require.NoError(t, env.data.CreateMessage(env.ctx, messagingRecord))

		resp, err = env.client.GetStatus(env.ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Exists)
		assert.True(t, resp.CodeScanned)
		assert.False(t, resp.IntentSubmitted)

		tc.intentRecord.IntentId = intentId.PublicKey().ToBase58()
		require.NoError(t, env.data.SaveIntent(env.ctx, tc.intentRecord))

		resp, err = env.client.GetStatus(env.ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Exists)
		assert.True(t, resp.CodeScanned)
		assert.True(t, resp.IntentSubmitted)
	}
}

func TestRegisterWebhook_HappyPath(t *testing.T) {
	paymentRequestRecord := &paymentrequest.Record{
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
	}
	loginRequestRecord := &paymentrequest.Record{
		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}

	for _, requestRecord := range []*paymentrequest.Record{
		paymentRequestRecord,
		loginRequestRecord,
	} {
		env, cleanup := setup(t)
		defer cleanup()

		intentId := testutil.NewRandomAccount(t)

		requestRecord.Intent = intentId.PublicKey().ToBase58()
		require.NoError(t, env.data.CreateRequest(env.ctx, requestRecord))

		registerReq := &micropaymentpb.RegisterWebhookRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
			Url: "https://getcode.com/webhook",
		}

		registerResp, err := env.client.RegisterWebhook(env.ctx, registerReq)
		require.NoError(t, err)
		assert.Equal(t, micropaymentpb.RegisterWebhookResponse_OK, registerResp.Result)

		webhookRecord, err := env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
		require.NoError(t, err)
		assert.Equal(t, intentId.PublicKey().ToBase58(), webhookRecord.WebhookId)
		assert.Equal(t, registerReq.Url, webhookRecord.Url)
		assert.Equal(t, webhook.TypeIntentSubmitted, webhookRecord.Type)
		assert.EqualValues(t, 0, webhookRecord.Attempts)
		assert.EqualValues(t, webhook.StateUnknown, webhookRecord.State)
		assert.Nil(t, webhookRecord.NextAttemptAt)
	}
}

func TestRegisterWebhook_NoPaymentRequest(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	intentId := testutil.NewRandomAccount(t)

	registerReq := &micropaymentpb.RegisterWebhookRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Url: "https://getcode.com/webhook",
	}

	registerResp, err := env.client.RegisterWebhook(env.ctx, registerReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.RegisterWebhookResponse_REQUEST_NOT_FOUND, registerResp.Result)

	_, err = env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, webhook.ErrNotFound, err)
}

func TestRegisterWebhook_IntentExists(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	intentId := testutil.NewRandomAccount(t)

	intentRecord := &intent.Record{
		IntentId:   intentId.PublicKey().ToBase58(),
		IntentType: intent.OpenAccounts,

		OpenAccountsMetadata: &intent.OpenAccountsMetadata{},

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
	}
	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))

	registerReq := &micropaymentpb.RegisterWebhookRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Url: "https://getcode.com/webhook",
	}

	registerResp, err := env.client.RegisterWebhook(env.ctx, registerReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.RegisterWebhookResponse_INTENT_EXISTS, registerResp.Result)

	_, err = env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, webhook.ErrNotFound, err)
}

func TestRegisterWebhook_AlreadyRegistered(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	intentId := testutil.NewRandomAccount(t)

	paymentRequestRecord := &paymentrequest.Record{
		Intent: intentId.PublicKey().ToBase58(),

		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
	}
	require.NoError(t, env.data.CreateRequest(env.ctx, paymentRequestRecord))

	for i := 0; i < 5; i++ {
		registerReq := &micropaymentpb.RegisterWebhookRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
			Url: fmt.Sprintf("https://getcode.com/webhook%d", i),
		}

		registerResp, err := env.client.RegisterWebhook(env.ctx, registerReq)
		require.NoError(t, err)
		if i == 0 {
			assert.Equal(t, micropaymentpb.RegisterWebhookResponse_OK, registerResp.Result)
		} else {
			assert.Equal(t, micropaymentpb.RegisterWebhookResponse_ALREADY_REGISTERED, registerResp.Result)
		}
	}

	webhookRecord, err := env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, intentId.PublicKey().ToBase58(), webhookRecord.WebhookId)
	assert.Equal(t, "https://getcode.com/webhook0", webhookRecord.Url)
	assert.Equal(t, webhook.TypeIntentSubmitted, webhookRecord.Type)
	assert.EqualValues(t, 0, webhookRecord.Attempts)
	assert.EqualValues(t, webhook.StateUnknown, webhookRecord.State)
	assert.Nil(t, webhookRecord.NextAttemptAt)
}

func TestRegisterWebhook_UrlValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	intentId := testutil.NewRandomAccount(t)

	paymentRequestRecord := &paymentrequest.Record{
		Intent: intentId.PublicKey().ToBase58(),

		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
	}
	require.NoError(t, env.data.CreateRequest(env.ctx, paymentRequestRecord))

	for _, invalidUrl := range baseInvalidUrlsToTest {
		registerReq := &micropaymentpb.RegisterWebhookRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
			Url: invalidUrl,
		}

		registerResp, err := env.client.RegisterWebhook(env.ctx, registerReq)
		if err != nil {
			testutil.AssertStatusErrorWithCode(t, err, codes.InvalidArgument)
		} else {
			require.NoError(t, err)
			assert.Equal(t, micropaymentpb.RegisterWebhookResponse_INVALID_URL, registerResp.Result)
		}
	}

	_, err := env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, webhook.ErrNotFound, err)
}

func TestCodify_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	destination := testutil.NewRandomAccount(t)

	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: owner.PublicKey().ToBase58(),
		TokenAccount:     destination.PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	codifyReq := &micropaymentpb.CodifyRequest{
		OwnerAccount:   owner.ToProto(),
		PrimaryAccount: destination.ToProto(),
		Currency:       "usd",
		NativeAmount:   0.25,
		Url:            "http://getcode.com",
	}

	reqBytes, err := proto.Marshal(codifyReq)
	require.NoError(t, err)

	codifyReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(owner.PrivateKey().ToBytes(), reqBytes),
	}

	codifyResp, err := env.client.Codify(env.ctx, codifyReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.CodifyResponse_OK, codifyResp.Result)
	assert.True(t, strings.HasPrefix(codifyResp.CodifiedUrl, codifiedContentUrlBase))
	assert.True(t, len(codifyResp.CodifiedUrl) > len(codifiedContentUrlBase)+4)

	shortPath := strings.Replace(codifyResp.CodifiedUrl, codifiedContentUrlBase, "", 1)

	paywallRecord, err := env.data.GetPaywallByShortPath(env.ctx, shortPath)
	require.NoError(t, err)
	assert.Equal(t, owner.PublicKey().ToBase58(), paywallRecord.OwnerAccount)
	assert.Equal(t, destination.PublicKey().ToBase58(), paywallRecord.DestinationTokenAccount)
	assert.EqualValues(t, codifyReq.Currency, paywallRecord.ExchangeCurrency)
	assert.Equal(t, codifyReq.NativeAmount, paywallRecord.NativeAmount)
	assert.Equal(t, codifyReq.Url, paywallRecord.RedirectUrl)
	assert.Equal(t, base58.Encode(codifyReq.Signature.Value), paywallRecord.Signature)

	getPathMetadataResp, err := env.client.GetPathMetadata(env.ctx, &micropaymentpb.GetPathMetadataRequest{
		Path: getRandomShortPath(),
	})
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.GetPathMetadataResponse_NOT_FOUND, getPathMetadataResp.Result)

	getPathMetadataResp, err = env.client.GetPathMetadata(env.ctx, &micropaymentpb.GetPathMetadataRequest{
		Path: shortPath,
	})
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.GetPathMetadataResponse_OK, getPathMetadataResp.Result)
	assert.Equal(t, destination.PublicKey().ToBytes(), getPathMetadataResp.Destination.Value)
	assert.Equal(t, codifyReq.Currency, getPathMetadataResp.Currency)
	assert.Equal(t, codifyReq.NativeAmount, getPathMetadataResp.NativeAmount)
	assert.Equal(t, codifyReq.Url, getPathMetadataResp.RedirctUrl)
}

func TestCodify_AccountValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	destination := testutil.NewRandomAccount(t)

	codifyReq := &micropaymentpb.CodifyRequest{
		OwnerAccount:   owner.ToProto(),
		PrimaryAccount: destination.ToProto(),
		Currency:       "usd",
		NativeAmount:   0.25,
		Url:            "http://getcode.com",
	}

	reqBytes, err := proto.Marshal(codifyReq)
	require.NoError(t, err)

	codifyReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(owner.PrivateKey().ToBytes(), reqBytes),
	}

	//
	// External accounts not allowed
	//

	codifyResp, err := env.client.Codify(env.ctx, codifyReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.CodifyResponse_INVALID_ACCOUNT, codifyResp.Result)
	assert.Empty(t, codifyResp.CodifiedUrl)

	//
	// Non-primary accounts not allowed
	//

	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		TokenAccount:     destination.PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_TEMPORARY_INCOMING,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	codifyResp, err = env.client.Codify(env.ctx, codifyReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.CodifyResponse_INVALID_ACCOUNT, codifyResp.Result)
	assert.Empty(t, codifyResp.CodifiedUrl)
}

func TestCodify_UrlValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	for _, invalidUrl := range append(
		baseInvalidUrlsToTest,
		"http://getcode.com/not-found",
	) {
		owner := testutil.NewRandomAccount(t)
		codifyReq := &micropaymentpb.CodifyRequest{
			OwnerAccount:   owner.ToProto(),
			PrimaryAccount: testutil.NewRandomAccount(t).ToProto(),
			Currency:       "usd",
			NativeAmount:   0.25,
			Url:            invalidUrl,
		}

		reqBytes, err := proto.Marshal(codifyReq)
		require.NoError(t, err)

		codifyReq.Signature = &commonpb.Signature{
			Value: ed25519.Sign(owner.PrivateKey().ToBytes(), reqBytes),
		}

		codifyResp, err := env.client.Codify(env.ctx, codifyReq)
		if err != nil {
			testutil.AssertStatusErrorWithCode(t, err, codes.InvalidArgument)
		} else {
			require.NoError(t, err)
			assert.Equal(t, micropaymentpb.CodifyResponse_INVALID_URL, codifyResp.Result)
			assert.Empty(t, codifyResp.CodifiedUrl)
		}
	}
}

func TestCodify_AmountValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	destination := testutil.NewRandomAccount(t)

	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: owner.PublicKey().ToBase58(),
		TokenAccount:     destination.PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	for _, amount := range []float64{0.01, 5.01} {
		codifyReq := &micropaymentpb.CodifyRequest{
			OwnerAccount:   owner.ToProto(),
			PrimaryAccount: destination.ToProto(),
			Currency:       "usd",
			NativeAmount:   amount,
			Url:            "http://getcode.com",
		}

		reqBytes, err := proto.Marshal(codifyReq)
		require.NoError(t, err)

		codifyReq.Signature = &commonpb.Signature{
			Value: ed25519.Sign(owner.PrivateKey().ToBytes(), reqBytes),
		}

		codifyResp, err := env.client.Codify(env.ctx, codifyReq)
		require.NoError(t, err)
		assert.Equal(t, micropaymentpb.CodifyResponse_NATIVE_AMOUNT_EXCEEDS_LIMIT, codifyResp.Result)
		assert.Empty(t, codifyResp.CodifiedUrl)
	}
}

func TestCodify_CurrencyValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	destination := testutil.NewRandomAccount(t)

	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: owner.PublicKey().ToBase58(),
		TokenAccount:     destination.PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	codifyReq := &micropaymentpb.CodifyRequest{
		OwnerAccount:   owner.ToProto(),
		PrimaryAccount: destination.ToProto(),
		Currency:       "btc",
		NativeAmount:   1,
		Url:            "http://getcode.com",
	}

	reqBytes, err := proto.Marshal(codifyReq)
	require.NoError(t, err)

	codifyReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(owner.PrivateKey().ToBytes(), reqBytes),
	}

	codifyResp, err := env.client.Codify(env.ctx, codifyReq)
	require.NoError(t, err)
	assert.Equal(t, micropaymentpb.CodifyResponse_UNSUPPORTED_CURRENCY, codifyResp.Result)
	assert.Empty(t, codifyResp.CodifiedUrl)
}

type testEnv struct {
	ctx    context.Context
	client micropaymentpb.MicroPaymentClient
	server *microPaymentServer
	data   code_data.Provider
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = micropaymentpb.NewMicroPaymentClient(conn)
	env.data = code_data.NewTestDataProvider()

	s := NewMicroPaymentServer(env.data, auth_util.NewRPCSignatureVerifier(env.data))
	env.server = s.(*microPaymentServer)

	serv.RegisterService(func(server *grpc.Server) {
		micropaymentpb.RegisterMicroPaymentServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}
