package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "auth.rpc_signature_verifier"
)

// RPCSignatureVerifier verifies signed requests messages by owner accounts.
type RPCSignatureVerifier struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewRPCSignatureVerifier(data code_data.Provider) *RPCSignatureVerifier {
	return &RPCSignatureVerifier{
		log:  logrus.StandardLogger().WithField("type", "auth/rpc_signature_verifier"),
		data: data,
	}
}

// Authenticate authenticates that a RPC request message is signed by the owner
// account public key.
func (v *RPCSignatureVerifier) Authenticate(ctx context.Context, owner *common.Account, message proto.Message, signature *commonpb.Signature) error {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "Authenticate").End()

	log := v.log.WithFields(logrus.Fields{
		"method":        "Authenticate",
		"owner_account": owner.PublicKey().ToBase58(),
	})

	isSignatureValid, err := v.isSignatureVerifiedProtoMessage(owner, message, signature)
	if err != nil {
		log.WithError(err).Warn("failure verifying signature")
		return status.Error(codes.Internal, "")
	}

	if !isSignatureValid {
		return status.Error(codes.Unauthenticated, "")
	}
	return nil
}

// marshalStrategy is a strategy for marshalling protobuf messages for signature
// verification
type marshalStrategy func(proto.Message) ([]byte, error)

// defaultMarshalStrategies are the default marshal strategies
var defaultMarshalStrategies = []marshalStrategy{
	forceConsistentMarshal,
	proto.Marshal, // todo: deprecate this option
}

func (v *RPCSignatureVerifier) isSignatureVerifiedProtoMessage(owner *common.Account, message proto.Message, signature *commonpb.Signature) (bool, error) {
	for _, marshalStrategy := range defaultMarshalStrategies {
		messageBytes, err := marshalStrategy(message)
		if err != nil {
			return false, err
		}

		isSignatureValid := ed25519.Verify(owner.PublicKey().ToBytes(), messageBytes, signature.Value)
		if isSignatureValid {
			return true, nil
		}
	}

	encoded, err := proto.Marshal(message)
	if err == nil {
		v.log.WithFields(logrus.Fields{
			"proto_message_type": message.ProtoReflect().Descriptor().FullName(),
			"proto_message":      base64.StdEncoding.EncodeToString(encoded),
			"signature":          base58.Encode(signature.Value),
		}).Info("proto message is not signature verified")
	}

	return false, nil
}
