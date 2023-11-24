package device

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	devicepb "github.com/code-payments/code-protobuf-api/generated/go/device/v1"

	"github.com/code-payments/code-server/pkg/testutil"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/phone"
)

func TestHappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	appInstallId := "app-install-id"
	owner := testutil.NewRandomAccount(t)

	env.setupUser(t, owner)

	getReq := &devicepb.GetLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: appInstallId,
		},
	}
	getResp, err := env.client.GetLoggedInAccounts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.GetLoggedInAccountsResponse_OK, getResp.Result)
	assert.Empty(t, getResp.Owners)

	registerReq := &devicepb.RegisterLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: appInstallId,
		},
		Owners: []*commonpb.SolanaAccountId{
			owner.ToProto(),
		},
	}
	registerReq.Signatures = []*commonpb.Signature{
		signProtoMessage(t, registerReq, owner, false),
	}

	registerResp, err := env.client.RegisterLoggedInAccounts(env.ctx, registerReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.RegisterLoggedInAccountsResponse_OK, registerResp.Result)
	assert.Empty(t, registerResp.InvalidOwners)

	getResp, err = env.client.GetLoggedInAccounts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.GetLoggedInAccountsResponse_OK, getResp.Result)
	require.Len(t, getResp.Owners, 1)
	assert.Equal(t, owner.PublicKey().ToBytes(), getResp.Owners[0].Value)

	registerReq = &devicepb.RegisterLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: appInstallId,
		},
	}
	registerResp, err = env.client.RegisterLoggedInAccounts(env.ctx, registerReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.RegisterLoggedInAccountsResponse_OK, registerResp.Result)
	assert.Empty(t, registerResp.InvalidOwners)

	getResp, err = env.client.GetLoggedInAccounts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.GetLoggedInAccountsResponse_OK, getResp.Result)
	assert.Empty(t, getResp.Owners)
}

func TestInvalidOwner(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	appInstallId := "app-install-id"
	owner := testutil.NewRandomAccount(t)

	registerReq := &devicepb.RegisterLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: appInstallId,
		},
		Owners: []*commonpb.SolanaAccountId{
			owner.ToProto(),
		},
	}
	registerReq.Signatures = []*commonpb.Signature{
		signProtoMessage(t, registerReq, owner, false),
	}

	registerResp, err := env.client.RegisterLoggedInAccounts(env.ctx, registerReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.RegisterLoggedInAccountsResponse_INVALID_OWNER, registerResp.Result)
	require.Len(t, registerResp.InvalidOwners, 1)
	assert.Equal(t, owner.PublicKey().ToBytes(), registerResp.InvalidOwners[0].Value)

	getReq := &devicepb.GetLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: appInstallId,
		},
	}
	getResp, err := env.client.GetLoggedInAccounts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, devicepb.GetLoggedInAccountsResponse_OK, getResp.Result)
	assert.Empty(t, getResp.Owners)
}

func TestUnauthorizedAccess(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)

	registerReq := &devicepb.RegisterLoggedInAccountsRequest{
		AppInstall: &commonpb.AppInstallId{
			Value: "app-install-id",
		},
		Owners: []*commonpb.SolanaAccountId{
			owner.ToProto(),
		},
	}
	registerReq.Signatures = []*commonpb.Signature{
		signProtoMessage(t, registerReq, owner, true),
	}

	_, err := env.client.RegisterLoggedInAccounts(env.ctx, registerReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

type testEnv struct {
	ctx    context.Context
	client devicepb.DeviceClient
	server *server
	data   code_data.Provider
}

func setup(t *testing.T) (env *testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env = &testEnv{
		ctx:    context.Background(),
		client: devicepb.NewDeviceClient(conn),
		data:   code_data.NewTestDataProvider(),
	}

	s := NewDeviceServer(env.data, auth_util.NewRPCSignatureVerifier(env.data))
	env.server = s.(*server)

	serv.RegisterService(func(server *grpc.Server) {
		devicepb.RegisterDeviceServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func (e *testEnv) setupUser(t *testing.T, owner *common.Account) {
	require.NoError(t, e.data.SavePhoneVerification(e.ctx, &phone.Verification{
		PhoneNumber:    "+12223334444",
		OwnerAccount:   owner.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))
}

func signProtoMessage(t *testing.T, msg proto.Message, signer *common.Account, simulateInvalidSignature bool) *commonpb.Signature {
	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err)

	if simulateInvalidSignature {
		signer = testutil.NewRandomAccount(t)
	}

	signature, err := signer.Sign(msgBytes)
	require.NoError(t, err)

	return &commonpb.Signature{
		Value: signature,
	}
}
