package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"

	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx      context.Context
	data     code_data.Provider
	verifier *RPCSignatureVerifier
}

func setup(t *testing.T) (env testEnv) {
	env.ctx = context.Background()
	env.data = code_data.NewTestDataProvider()
	env.verifier = NewRPCSignatureVerifier(env.data)
	return env
}

func TestAuthenticate(t *testing.T) {
	env := setup(t)

	ownerAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	msgValue, _ := uuid.New().MarshalBinary()
	msg := &messagingpb.MessageId{
		Value: msgValue,
	}

	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(msgBytes)
	require.NoError(t, err)
	signatureProto := &commonpb.Signature{
		Value: signature,
	}

	err = env.verifier.Authenticate(env.ctx, ownerAccount, msg, signatureProto)
	require.NoError(t, err)

	signature, err = maliciousAccount.Sign(msgBytes)
	require.NoError(t, err)
	signatureProto = &commonpb.Signature{
		Value: signature,
	}

	err = env.verifier.Authenticate(env.ctx, ownerAccount, msg, signatureProto)
	assert.Error(t, err)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func TestAuthorizeDataAccess(t *testing.T) {
	env := setup(t)

	dataContainerID := user.NewDataContainerID()
	phoneNumber := "+11234567890"

	ownerAccount := testutil.NewRandomAccount(t)

	maliciousAccount := testutil.NewRandomAccount(t)

	msgValue, _ := uuid.New().MarshalBinary()
	msg := &messagingpb.MessageId{
		Value: msgValue,
	}

	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(msgBytes)
	require.NoError(t, err)
	signatureProto := &commonpb.Signature{
		Value: signature,
	}

	// Data container doesn't exist
	err = env.verifier.AuthorizeDataAccess(env.ctx, dataContainerID, ownerAccount, msg, signatureProto)
	assert.Error(t, err)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	require.NoError(t, env.data.PutUserDataContainer(env.ctx, &storage.Record{
		ID:           dataContainerID,
		OwnerAccount: ownerAccount.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}))

	// Successful authorization
	err = env.verifier.AuthorizeDataAccess(env.ctx, dataContainerID, ownerAccount, msg, signatureProto)
	assert.NoError(t, err)

	signature, err = maliciousAccount.Sign(msgBytes)
	require.NoError(t, err)
	signatureProto = &commonpb.Signature{
		Value: signature,
	}

	// Token account doesn't own data container
	err = env.verifier.AuthorizeDataAccess(env.ctx, dataContainerID, maliciousAccount, msg, signatureProto)
	assert.Error(t, err)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	// Signature doesn't match public key
	err = env.verifier.AuthorizeDataAccess(env.ctx, dataContainerID, ownerAccount, msg, signatureProto)
	assert.Error(t, err)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}
