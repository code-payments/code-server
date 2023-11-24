package push

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/code-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	memory_push "github.com/code-payments/code-server/pkg/push/memory"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestAddToken_HappyPath_AndroidToken(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newAndroidClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	req := makeAddFcmAndroidTokenReq(t, ownerAccount, *containerID)
	resp, err := client.AddToken(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, pushpb.AddTokenResponse_OK, resp.Result)

	records, err := env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	require.NoError(t, err)
	require.Len(t, records, 1)

	record := records[0]
	assert.Equal(t, req.PushToken, record.PushToken)
	assert.Equal(t, push.TokenTypeFcmAndroid, record.TokenType)
	assert.True(t, record.IsValid)
	require.NotNil(t, record.AppInstallId)
	assert.Equal(t, req.AppInstall.Value, *record.AppInstallId)
}

func TestAddToken_HappyPath_APNSToken(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newIOSClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	req := makeAddFcmApnsTokenReq(t, ownerAccount, *containerID)
	resp, err := client.AddToken(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, pushpb.AddTokenResponse_OK, resp.Result)

	records, err := env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	require.NoError(t, err)
	require.Len(t, records, 1)

	record := records[0]
	assert.Equal(t, req.PushToken, record.PushToken)
	assert.Equal(t, push.TokenTypeFcmApns, record.TokenType)
	assert.True(t, record.IsValid)
	require.NotNil(t, record.AppInstallId)
	assert.Equal(t, req.AppInstall.Value, *record.AppInstallId)
}

func TestAddToken_HappyPath_MultipleTokens(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	androidClient := newAndroidClient(t, env)
	iosClient := newIOSClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	androidTokenReq := makeAddFcmAndroidTokenReq(t, ownerAccount, *containerID)
	resp, err := androidClient.AddToken(env.ctx, androidTokenReq)
	require.NoError(t, err)
	assert.Equal(t, pushpb.AddTokenResponse_OK, resp.Result)

	apnsTokenReq := makeAddFcmApnsTokenReq(t, ownerAccount, *containerID)
	resp, err = iosClient.AddToken(env.ctx, apnsTokenReq)
	require.NoError(t, err)
	assert.Equal(t, pushpb.AddTokenResponse_OK, resp.Result)

	records, err := env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	require.NoError(t, err)
	require.Len(t, records, 2)

	record := records[0]
	assert.Equal(t, androidTokenReq.PushToken, record.PushToken)
	assert.Equal(t, push.TokenTypeFcmAndroid, record.TokenType)
	assert.True(t, record.IsValid)
	require.NotNil(t, record.AppInstallId)
	assert.Equal(t, androidTokenReq.AppInstall.Value, *record.AppInstallId)

	record = records[1]
	assert.Equal(t, apnsTokenReq.PushToken, record.PushToken)
	assert.Equal(t, push.TokenTypeFcmApns, record.TokenType)
	assert.True(t, record.IsValid)
	require.NotNil(t, record.AppInstallId)
	assert.Equal(t, apnsTokenReq.AppInstall.Value, *record.AppInstallId)
}

func TestAddToken_InvalidToken(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newAndroidClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	invalidTokenReq := makeAddReqWithInvalidToken(t, ownerAccount, *containerID)
	resp, err := client.AddToken(env.ctx, invalidTokenReq)
	require.NoError(t, err)
	assert.Equal(t, pushpb.AddTokenResponse_INVALID_PUSH_TOKEN, resp.Result)

	_, err = env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	assert.Equal(t, push.ErrTokenNotFound, err)
}

func TestAddToken_UserAgentValidation(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	androidTokenReq := makeAddFcmAndroidTokenReq(t, ownerAccount, *containerID)
	apnsTokenReq := makeAddFcmApnsTokenReq(t, ownerAccount, *containerID)

	// No user-agent header value
	client := newClientWithoutUserAgent(t, env)
	_, err := client.AddToken(env.ctx, androidTokenReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.InvalidArgument)

	// Android client setting an APNS push token
	client = newAndroidClient(t, env)
	_, err = client.AddToken(env.ctx, apnsTokenReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.InvalidArgument)

	// iOS client setting an Android push token
	client = newIOSClient(t, env)
	_, err = client.AddToken(env.ctx, androidTokenReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.InvalidArgument)

	// No push tokens should be saved
	_, err = env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	assert.Equal(t, push.ErrTokenNotFound, err)
}

func TestAddToken_Idempotency(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newAndroidClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	invalidTokenReq := makeAddFcmAndroidTokenReq(t, ownerAccount, *containerID)
	for i := 0; i < 5; i++ {
		resp, err := client.AddToken(env.ctx, invalidTokenReq)
		require.NoError(t, err)
		assert.Equal(t, pushpb.AddTokenResponse_OK, resp.Result)
	}

	records, err := env.data.GetAllValidPushTokensdByDataContainer(env.ctx, containerID)
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newAndroidClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)

	maliciousAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	req := &pushpb.AddTokenRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		PushToken:      memory_push.ValidAndroidPushToken,
		TokenType:      pushpb.TokenType_FCM_ANDROID,
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	_, err = client.AddToken(env.ctx, req)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func TestUnauthorizedDataAccess(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	client := newAndroidClient(t, env)

	ownerAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount)

	req := &pushpb.AddTokenRequest{
		OwnerAccountId: maliciousAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		PushToken:      memory_push.ValidAndroidPushToken,
		TokenType:      pushpb.TokenType_FCM_ANDROID,
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	_, err = client.AddToken(env.ctx, req)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)
}

type testEnv struct {
	ctx    context.Context
	target string
	server *pushServer
	data   code_data.Provider
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.target = conn.Target()
	env.data = code_data.NewTestDataProvider()

	s := NewPushServer(env.data, auth.NewRPCSignatureVerifier(env.data), memory_push.NewPushProvider())
	env.server = s.(*pushServer)

	serv.RegisterService(func(server *grpc.Server) {
		pushpb.RegisterPushServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func makeAddFcmAndroidTokenReq(t *testing.T, ownerAccount *common.Account, containerID user.DataContainerID) *pushpb.AddTokenRequest {
	req := &pushpb.AddTokenRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		PushToken:      memory_push.ValidAndroidPushToken,
		AppInstall: &commonpb.AppInstallId{
			Value: uuid.NewString(),
		},
		TokenType: pushpb.TokenType_FCM_ANDROID,
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	return req
}

func makeAddFcmApnsTokenReq(t *testing.T, ownerAccount *common.Account, containerID user.DataContainerID) *pushpb.AddTokenRequest {
	req := &pushpb.AddTokenRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		PushToken:      memory_push.ValidApplePushToken,
		AppInstall: &commonpb.AppInstallId{
			Value: uuid.NewString(),
		},
		TokenType: pushpb.TokenType_FCM_APNS,
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	return req
}

func makeAddReqWithInvalidToken(t *testing.T, ownerAccount *common.Account, containerID user.DataContainerID) *pushpb.AddTokenRequest {
	req := &pushpb.AddTokenRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		PushToken:      memory_push.InvalidPushToken,
		TokenType:      pushpb.TokenType_FCM_ANDROID,
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	return req
}

func generateNewDataContainer(t *testing.T, env testEnv, ownerAccount *common.Account) *user.DataContainerID {
	phoneNumber := "+12223334444"

	container := &storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: ownerAccount.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUserDataContainer(env.ctx, container))
	return container.ID
}

// todo: integrate below client utilities with the main testutil package

func newClientWithoutUserAgent(t *testing.T, env testEnv) pushpb.PushClient {
	conn, err := grpc.Dial(env.target, grpc.WithInsecure())
	require.NoError(t, err)

	return pushpb.NewPushClient(conn)
}

func newAndroidClient(t *testing.T, env testEnv) pushpb.PushClient {
	conn, err := grpc.Dial(env.target, grpc.WithInsecure(), grpc.WithUserAgent("Code/Android/1.0.0"))
	require.NoError(t, err)

	return pushpb.NewPushClient(conn)
}

func newIOSClient(t *testing.T, env testEnv) pushpb.PushClient {
	conn, err := grpc.Dial(env.target, grpc.WithInsecure(), grpc.WithUserAgent("Code/iOS/1.0.0"))
	require.NoError(t, err)

	return pushpb.NewPushClient(conn)
}
