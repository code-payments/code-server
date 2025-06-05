package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"

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
	for _, marshalStrategy := range defaultMarshalStrategies {
		env := setup(t)

		ownerAccount := testutil.NewRandomAccount(t)
		maliciousAccount := testutil.NewRandomAccount(t)

		msgValue, _ := uuid.New().MarshalBinary()
		msg := &messagingpb.MessageId{
			Value: msgValue,
		}

		msgBytes, err := marshalStrategy(msg)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(msgBytes)
		require.NoError(t, err)
		signatureProto := &commonpb.Signature{
			Value: signature,
		}

		err = env.verifier.Authenticate(env.ctx, ownerAccount, msg, signatureProto)
		require.NoError(t, err)

		err = env.verifier.Authenticate(env.ctx, ownerAccount, msg, nil)
		testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

		signature, err = maliciousAccount.Sign(msgBytes)
		require.NoError(t, err)
		signatureProto = &commonpb.Signature{
			Value: signature,
		}

		err = env.verifier.Authenticate(env.ctx, ownerAccount, msg, signatureProto)
		assert.Error(t, err)
		testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
	}
}
