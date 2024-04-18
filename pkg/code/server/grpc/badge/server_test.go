package badge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	badgepb "github.com/code-payments/code-protobuf-api/generated/go/badge/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/badgecount"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"
	memory_push "github.com/code-payments/code-server/pkg/push/memory"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestResetBadgeCount_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)

	env.createUser(t, owner, "+12223334444")

	req := &badgepb.ResetBadgeCountRequest{
		Owner: owner.ToProto(),
	}
	req.Signature = signProtoMessage(t, req, owner, false)

	resp, err := env.client.ResetBadgeCount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, resp.Result, badgepb.ResetBadgeCountResponse_OK)
	env.assertBadgeCount(t, owner, 0)

	require.NoError(t, env.data.AddToBadgeCount(env.ctx, owner.PublicKey().ToBase58(), 5))
	env.assertBadgeCount(t, owner, 5)

	resp, err = env.client.ResetBadgeCount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, resp.Result, badgepb.ResetBadgeCountResponse_OK)
	env.assertBadgeCount(t, owner, 0)
}

func TestUnauthorizedAccess(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)

	resetReq := &badgepb.ResetBadgeCountRequest{
		Owner: owner.ToProto(),
	}
	resetReq.Signature = signProtoMessage(t, resetReq, owner, true)

	_, err := env.client.ResetBadgeCount(env.ctx, resetReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

type testEnv struct {
	ctx    context.Context
	client badgepb.BadgeClient
	server *server
	data   code_data.Provider
}

func setup(t *testing.T) (env *testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env = &testEnv{
		ctx:    context.Background(),
		client: badgepb.NewBadgeClient(conn),
		data:   code_data.NewTestDataProvider(),
	}

	s := NewBadgeServer(env.data, memory_push.NewPushProvider(), auth_util.NewRPCSignatureVerifier(env.data))
	env.server = s.(*server)

	serv.RegisterService(func(server *grpc.Server) {
		badgepb.RegisterBadgeServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func (e *testEnv) createUser(t *testing.T, owner *common.Account, phoneNumber string) {
	phoneVerificationRecord := &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   owner.PublicKey().ToBase58(),
		LastVerifiedAt: time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, e.data.SavePhoneVerification(e.ctx, phoneVerificationRecord))

	userIdentityRecord := &user_identity.Record{
		ID: user.NewID(),
		View: &user.View{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, e.data.PutUser(e.ctx, userIdentityRecord))

	userStorageRecord := &user_storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: owner.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, e.data.PutUserDataContainer(e.ctx, userStorageRecord))

	pushTokenRecord := push.Record{
		DataContainerId: *userStorageRecord.ID,

		PushToken: memory_push.ValidApplePushToken,
		TokenType: push.TokenTypeFcmApns,
		IsValid:   true,

		CreatedAt: time.Now(),
	}
	require.NoError(t, e.data.PutPushToken(e.ctx, &pushTokenRecord))
}

func (e *testEnv) assertBadgeCount(t *testing.T, owner *common.Account, expected int) {
	badgeCountRecord, err := e.data.GetBadgeCount(e.ctx, owner.PublicKey().ToBase58())
	if err == badgecount.ErrBadgeCountNotFound {
		assert.Equal(t, 0, expected)
		return
	}
	require.NoError(t, err)
	assert.EqualValues(t, expected, badgeCountRecord.BadgeCount)
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
