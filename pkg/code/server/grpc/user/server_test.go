package user

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx    context.Context
	client userpb.IdentityClient
	server *identityServer
	data   code_data.Provider
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	// Force iOS user agent to pass airdrop tests
	iosConn, err := grpc.Dial(conn.Target(), grpc.WithInsecure(), grpc.WithUserAgent("Code/iOS/1.0.0"))
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = userpb.NewIdentityClient(iosConn)
	env.data = code_data.NewTestDataProvider()

	s := NewIdentityServer(
		env.data,
		auth.NewRPCSignatureVerifier(env.data),
		messaging.NewMessagingClient(env.data),
	)
	env.server = s.(*identityServer)
	env.server.domainVerifier = mockDomainVerifier

	testutil.SetupRandomSubsidizer(t, env.data)

	serv.RegisterService(func(server *grpc.Server) {
		userpb.RegisterIdentityServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

/*
func TestUpdatePreferences_Locale_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	containerId := generateNewDataContainer(t, env, ownerAccount)

	for _, expected := range []string{
		"fr",
		"fr-CA",
		"fr_CA",
		"en",
		"en-US",
		"en_US",
		"mni-Beng_IN",
	} {
		req := &userpb.UpdatePreferencesRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			ContainerId:    containerId.Proto(),
			Locale: &commonpb.Locale{
				Value: expected,
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		resp, err := env.client.UpdatePreferences(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, userpb.UpdatePreferencesResponse_OK, resp.Result)

		preferencesRecord, err := env.data.GetUserPreferences(env.ctx, containerId)
		require.NoError(t, err)
		assert.Equal(t, strings.Replace(expected, "_", "-", -1), preferencesRecord.Locale.String())
	}
}

func TestUpdatePreferences_Locale_InvalidLocale(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	containerId := generateNewDataContainer(t, env, ownerAccount)

	for _, invalidValue := range []string{
		"zz",
	} {
		req := &userpb.UpdatePreferencesRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			ContainerId:    containerId.Proto(),
			Locale: &commonpb.Locale{
				Value: invalidValue,
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(ownerAccount.PrivateKey().ToBytes(), reqBytes),
		}

		resp, err := env.client.UpdatePreferences(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, userpb.UpdatePreferencesResponse_INVALID_LOCALE, resp.Result)

		_, err = env.data.GetUserPreferences(env.ctx, containerId)
		assert.Equal(t, preferences.ErrPreferencesNotFound, err)
	}
}
*/

func TestLoginToThirdPartyApp_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	relationshipAuthorityAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: relationshipAuthorityAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(relationshipAuthorityAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:     intentId.PublicKey().ToBase58(),
		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}))

	require.NoError(t, env.data.CreateWebhook(env.ctx, &webhook.Record{
		WebhookId: intentId.PublicKey().ToBase58(),
		Url:       "example.com/webhook",
		Type:      webhook.TypeIntentSubmitted,
		State:     webhook.StateUnknown,
	}))

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		RelationshipTo: pointer.String("example.com"),
	}))

	resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_OK, resp.Result)

	intentRecord, err := env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Equal(t, intentId.PublicKey().ToBase58(), intentRecord.IntentId)
	assert.Equal(t, intent.Login, intentRecord.IntentType)
	require.NotNil(t, intentRecord.LoginMetadata)
	assert.Equal(t, "example.com", intentRecord.LoginMetadata.App)
	assert.Equal(t, relationshipAuthorityAccount.PublicKey().ToBase58(), intentRecord.LoginMetadata.UserId)
	assert.Equal(t, ownerAccount.PublicKey().ToBase58(), intentRecord.InitiatorOwnerAccount)
	assert.Equal(t, intent.StateConfirmed, intentRecord.State)

	webhookRecord, err := env.data.GetWebhook(env.ctx, intentId.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.True(t, webhookRecord.NextAttemptAt.Before(time.Now()))
	assert.Equal(t, webhook.StatePending, webhookRecord.State)

	messageRecords, err := env.data.GetMessages(env.ctx, intentId.PublicKey().ToBase58())
	require.NoError(t, err)
	require.Len(t, messageRecords, 1)

	var protoMessage messagingpb.Message
	require.NoError(t, proto.Unmarshal(messageRecords[0].Message, &protoMessage))
	require.NotNil(t, protoMessage.GetIntentSubmitted())
	assert.Equal(t, intentId.PublicKey().ToBytes(), protoMessage.GetIntentSubmitted().IntentId.Value)
	assert.Nil(t, protoMessage.GetIntentSubmitted().Metadata)

	resp, err = env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_OK, resp.Result)
}

func TestLoginToThirdPartyApp_RequestNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	relationshipAuthorityAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: relationshipAuthorityAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(relationshipAuthorityAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		RelationshipTo: pointer.String("example.com"),
	}))

	resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_REQUEST_NOT_FOUND, resp.Result)

	_, err = env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func TestLoginToThirdPartyApp_MultipleUsers(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	relationshipAuthorityAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: relationshipAuthorityAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(relationshipAuthorityAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:     intentId.PublicKey().ToBase58(),
		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}))

	require.NoError(t, env.data.SaveIntent(env.ctx, &intent.Record{
		IntentId:   intentId.PublicKey().ToBase58(),
		IntentType: intent.Login,

		LoginMetadata: &intent.LoginMetadata{
			App:    "example.com",
			UserId: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		},

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		State:                 intent.StateConfirmed,
	}))

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		RelationshipTo: pointer.String("example.com"),
	}))

	resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_DIFFERENT_LOGIN_EXISTS, resp.Result)

	intentRecord, err := env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.NotEqual(t, intentRecord.InitiatorOwnerAccount, ownerAccount.PublicKey().ToBase58())
	assert.NotEqual(t, intentRecord.LoginMetadata.UserId, relationshipAuthorityAccount.PublicKey().ToBase58())
}

func TestLoginToThirdPartyApp_InvalidAccount(t *testing.T) {
	for _, accountType := range []commonpb.AccountType{
		commonpb.AccountType_RELATIONSHIP,
		commonpb.AccountType_PRIMARY,
		commonpb.AccountType_BUCKET_100_KIN,
		commonpb.AccountType_TEMPORARY_INCOMING,
		commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
	} {
		env, cleanup := setup(t)
		defer cleanup()

		ownerAccount := testutil.NewRandomAccount(t)
		authorityAccount := testutil.NewRandomAccount(t)

		intentId := testutil.NewRandomAccount(t)

		req := &userpb.LoginToThirdPartyAppRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
			UserId: authorityAccount.ToProto(),
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(authorityAccount.PrivateKey().ToBytes(), reqBytes),
		}

		require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
			Intent:     intentId.PublicKey().ToBase58(),
			Domain:     pointer.String("app1.com"),
			IsVerified: true,
		}))

		accountInfoRecord := &account.Record{
			OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
			AuthorityAccount: authorityAccount.PublicKey().ToBase58(),
			TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
			AccountType:      accountType,
		}
		if accountType == commonpb.AccountType_PRIMARY || accountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
			accountInfoRecord.OwnerAccount = authorityAccount.PublicKey().ToBase58()
			ownerAccount = authorityAccount
		}
		if accountType == commonpb.AccountType_RELATIONSHIP {
			accountInfoRecord.RelationshipTo = pointer.String("app2.com")
		}
		require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

		resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, userpb.LoginToThirdPartyAppResponse_INVALID_ACCOUNT, resp.Result)

		_, err = env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
		assert.Equal(t, intent.ErrIntentNotFound, err)
	}
}

func TestLoginToThirdPartyApp_LoginNotSupported(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	relationshipAuthorityAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: relationshipAuthorityAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(relationshipAuthorityAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:                  intentId.PublicKey().ToBase58(),
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.00),
	}))

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		RelationshipTo: pointer.String("example.com"),
	}))

	resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_LOGIN_NOT_SUPPORTED, resp.Result)

	_, err = env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func TestLoginToThirdPartyApp_PaymentRequired(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	relationshipAuthorityAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: relationshipAuthorityAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(relationshipAuthorityAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:                  intentId.PublicKey().ToBase58(),
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.00),
		Domain:                  pointer.String("example.com"),
		IsVerified:              true,
	}))

	require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
		TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

		AccountType:    commonpb.AccountType_RELATIONSHIP,
		RelationshipTo: pointer.String("example.com"),
	}))

	resp, err := env.client.LoginToThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.LoginToThirdPartyAppResponse_PAYMENT_REQUIRED, resp.Result)

	_, err = env.data.GetIntent(env.ctx, intentId.PublicKey().ToBase58())
	assert.Equal(t, intent.ErrIntentNotFound, err)
}

func TestGetLoginForThirdPartyApp_HappyPath(t *testing.T) {
	paymentRequestRecord := &paymentrequest.Record{
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
		Domain:                  pointer.String("example.com"),
		IsVerified:              true,
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

			IsMicroPayment: true,
			IsWithdrawal:   true,
		},

		State: intent.StatePending,
	}

	loginIntentRecord := &intent.Record{
		IntentType: intent.Login,

		LoginMetadata: &intent.LoginMetadata{
			App:    "example.com",
			UserId: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		},

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

		verifierAccount, err := common.NewAccountFromPrivateKeyString("dr2MUzL4NCS45qyp16vDXiSdHqqdg2DF79xKaYMB1vzVtDDjPvyQ8xTH4VsTWXSDP3NFzsdCV6gEoChKftzwLno")
		require.NoError(t, err)

		intentId := testutil.NewRandomAccount(t)

		ownerAccount := testutil.NewRandomAccount(t)
		relationshipAuthorityAccount := testutil.NewRandomAccount(t)

		require.NoError(t, env.data.CreateAccountInfo(env.ctx, &account.Record{
			OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
			AuthorityAccount: relationshipAuthorityAccount.PublicKey().ToBase58(),
			TokenAccount:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),

			AccountType:    commonpb.AccountType_RELATIONSHIP,
			RelationshipTo: pointer.String("example.com"),
		}))

		req := &userpb.GetLoginForThirdPartyAppRequest{
			IntentId: &commonpb.IntentId{
				Value: intentId.ToProto().Value,
			},
			Verifier: verifierAccount.ToProto(),
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		req.Signature = &commonpb.Signature{
			Value: ed25519.Sign(verifierAccount.PrivateKey().ToBytes(), reqBytes),
		}

		tc.requestRecord.Intent = intentId.PublicKey().ToBase58()
		require.NoError(t, env.data.CreateRequest(env.ctx, tc.requestRecord))

		resp, err := env.client.GetLoginForThirdPartyApp(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, userpb.GetLoginForThirdPartyAppResponse_NO_USER_LOGGED_IN, resp.Result)
		assert.Nil(t, resp.UserId)

		tc.intentRecord.IntentId = intentId.PublicKey().ToBase58()
		tc.intentRecord.InitiatorOwnerAccount = ownerAccount.PublicKey().ToBase58()
		require.NoError(t, env.data.SaveIntent(env.ctx, tc.intentRecord))

		resp, err = env.client.GetLoginForThirdPartyApp(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, userpb.GetLoginForThirdPartyAppResponse_OK, resp.Result)
		require.NotNil(t, resp.UserId)
		assert.Equal(t, relationshipAuthorityAccount.PublicKey().ToBytes(), resp.UserId.Value)
	}
}

func TestGetLoginForThirdPartyApp_RequestNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	verifierAccount, err := common.NewAccountFromPrivateKeyString("dr2MUzL4NCS45qyp16vDXiSdHqqdg2DF79xKaYMB1vzVtDDjPvyQ8xTH4VsTWXSDP3NFzsdCV6gEoChKftzwLno")
	require.NoError(t, err)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.GetLoginForThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Verifier: verifierAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(verifierAccount.PrivateKey().ToBytes(), reqBytes),
	}

	resp, err := env.client.GetLoginForThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetLoginForThirdPartyAppResponse_REQUEST_NOT_FOUND, resp.Result)
	assert.Nil(t, resp.UserId)
}

func TestGetLoginForThirdPartyApp_LoginNotSupported(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	verifierAccount, err := common.NewAccountFromPrivateKeyString("dr2MUzL4NCS45qyp16vDXiSdHqqdg2DF79xKaYMB1vzVtDDjPvyQ8xTH4VsTWXSDP3NFzsdCV6gEoChKftzwLno")
	require.NoError(t, err)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.GetLoginForThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Verifier: verifierAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(verifierAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:                  intentId.PublicKey().ToBase58(),
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
		Domain:                  pointer.String("example.com"),
		IsVerified:              false,
	}))

	resp, err := env.client.GetLoginForThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetLoginForThirdPartyAppResponse_LOGIN_NOT_SUPPORTED, resp.Result)
	assert.Nil(t, resp.UserId)
}

func TestGetLoginForThirdPartyApp_RelationshipNotEstablished(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	verifierAccount, err := common.NewAccountFromPrivateKeyString("dr2MUzL4NCS45qyp16vDXiSdHqqdg2DF79xKaYMB1vzVtDDjPvyQ8xTH4VsTWXSDP3NFzsdCV6gEoChKftzwLno")
	require.NoError(t, err)

	intentId := testutil.NewRandomAccount(t)

	req := &userpb.GetLoginForThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Verifier: verifierAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(verifierAccount.PrivateKey().ToBytes(), reqBytes),
	}

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:                  intentId.PublicKey().ToBase58(),
		DestinationTokenAccount: pointer.String(testutil.NewRandomAccount(t).PrivateKey().ToBase58()),
		ExchangeCurrency:        pointer.String(string(currency_lib.USD)),
		NativeAmount:            pointer.Float64(1.0),
		Domain:                  pointer.String("example.com"),
		IsVerified:              true,
	}))

	require.NoError(t, env.data.SaveIntent(env.ctx, &intent.Record{
		IntentId:   intentId.PublicKey().ToBase58(),
		IntentType: intent.SendPrivatePayment,

		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			ExchangeCurrency: currency_lib.USD,
			NativeAmount:     1.0,
			ExchangeRate:     0.1,
			Quantity:         kin.ToQuarks(10),
			UsdMarketValue:   1.0,

			DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

			IsMicroPayment: true,
			IsWithdrawal:   true,
		},

		InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		State:                 intent.StatePending,
	}))

	resp, err := env.client.GetLoginForThirdPartyApp(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetLoginForThirdPartyAppResponse_NO_USER_LOGGED_IN, resp.Result)
	assert.Nil(t, resp.UserId)
}

func TestGetTwitterUser_ByUsername_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	req := &userpb.GetTwitterUserRequest{
		Query: &userpb.GetTwitterUserRequest_Username{
			Username: "jeffyanta",
		},
	}
	resp, err := env.client.GetTwitterUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetTwitterUserResponse_NOT_FOUND, resp.Result)
	assert.Nil(t, resp.TwitterUser)

	record := &twitter.Record{
		Username:      req.GetUsername(),
		Name:          "Jeff",
		ProfilePicUrl: "https://pbs.twimg.com/profile_images/1728595562285441024/GM-aLyh__normal.jpg",
		VerifiedType:  userpb.TwitterUser_BLUE,
		FollowerCount: 200,
		TipAddress:    testutil.NewRandomAccount(t).PublicKey().ToBase58(),
	}
	require.NoError(t, env.data.SaveTwitterUser(env.ctx, record))

	resp, err = env.client.GetTwitterUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetTwitterUserResponse_OK, resp.Result)
	assert.Equal(t, record.TipAddress, base58.Encode(resp.TwitterUser.TipAddress.Value))
	assert.Equal(t, record.Username, resp.TwitterUser.Username)
	assert.Equal(t, record.Name, resp.TwitterUser.Name)
	assert.Equal(t, record.ProfilePicUrl, resp.TwitterUser.ProfilePicUrl)
	assert.Equal(t, record.VerifiedType, resp.TwitterUser.VerifiedType)
	assert.Equal(t, record.FollowerCount, resp.TwitterUser.FollowerCount)
}

func TestGetTwitterUser_ByTipAddress_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	tipAddress := testutil.NewRandomAccount(t)

	req := &userpb.GetTwitterUserRequest{
		Query: &userpb.GetTwitterUserRequest_TipAddress{
			TipAddress: tipAddress.ToProto(),
		},
	}
	resp, err := env.client.GetTwitterUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetTwitterUserResponse_NOT_FOUND, resp.Result)
	assert.Nil(t, resp.TwitterUser)

	record := &twitter.Record{
		Username:      "jeffyanta",
		Name:          "Jeff",
		ProfilePicUrl: "https://pbs.twimg.com/profile_images/1728595562285441024/GM-aLyh__normal.jpg",
		VerifiedType:  userpb.TwitterUser_BLUE,
		FollowerCount: 200,
		TipAddress:    tipAddress.PublicKey().ToBase58(),
	}
	require.NoError(t, env.data.SaveTwitterUser(env.ctx, record))

	resp, err = env.client.GetTwitterUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetTwitterUserResponse_OK, resp.Result)
	assert.Equal(t, record.TipAddress, base58.Encode(resp.TwitterUser.TipAddress.Value))
	assert.Equal(t, record.Username, resp.TwitterUser.Username)
	assert.Equal(t, record.Name, resp.TwitterUser.Name)
	assert.Equal(t, record.ProfilePicUrl, resp.TwitterUser.ProfilePicUrl)
	assert.Equal(t, record.VerifiedType, resp.TwitterUser.VerifiedType)
	assert.Equal(t, record.FollowerCount, resp.TwitterUser.FollowerCount)
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	validAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	intentId := testutil.NewRandomAccount(t)

	require.NoError(t, env.data.CreateRequest(env.ctx, &paymentrequest.Record{
		Intent:     intentId.PublicKey().ToBase58(),
		Domain:     pointer.String("example.com"),
		IsVerified: true,
	}))

	/*
		updatePreferencesReq := &userpb.UpdatePreferencesRequest{
			OwnerAccountId: validAccount.ToProto(),
			ContainerId:    containerId.Proto(),
			Locale: &commonpb.Locale{
				Value: language.CanadianFrench.String(),
			},
		}

		reqBytes, err := proto.Marshal(updatePreferencesReq)
		require.NoError(t, err)

		updatePreferencesReq.Signature = &commonpb.Signature{
			Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
		}
	*/

	loginReq := &userpb.LoginToThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		UserId: validAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(loginReq)
	require.NoError(t, err)

	loginReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	getLoginReq := &userpb.GetLoginForThirdPartyAppRequest{
		IntentId: &commonpb.IntentId{
			Value: intentId.ToProto().Value,
		},
		Verifier: testutil.NewRandomAccount(t).ToProto(),
	}

	reqBytes, err = proto.Marshal(getLoginReq)
	require.NoError(t, err)

	getLoginReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	/*
		_, err = env.client.UpdatePreferences(env.ctx, updatePreferencesReq)
		testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
	*/

	_, err = env.client.LoginToThirdPartyApp(env.ctx, loginReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	_, err = env.client.GetLoginForThirdPartyApp(env.ctx, getLoginReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func mockDomainVerifier(ctx context.Context, owner *common.Account, domain string) (bool, error) {
	// Private key: dr2MUzL4NCS45qyp16vDXiSdHqqdg2DF79xKaYMB1vzVtDDjPvyQ8xTH4VsTWXSDP3NFzsdCV6gEoChKftzwLno
	return owner.PublicKey().ToBase58() == "AiXmGd1DkRbVyfiLLNxC6EFF9ZidCdGpyVY9QFH966Bm", nil
}
