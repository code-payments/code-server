package request

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/solana"
)

// todo: Migrate to a generic HTTP -> gRPC with signed proto strategy

type trustlessRequest struct {
	originalProtoMessage *messagingpb.Message
	publicRendezvousKey  *common.Account
	clientSignature      solana.Signature // For a messagingpb.RequestToReceiveBill
	webhookUrl           *string
}

func newTrustlessRequest(
	originalProtoMessage *messagingpb.Message,
	publicRendezvousKey *common.Account,
	clientSignature solana.Signature,
	webhookUrl *string,
) (*trustlessRequest, error) {
	return &trustlessRequest{
		originalProtoMessage: originalProtoMessage,
		publicRendezvousKey:  publicRendezvousKey,
		clientSignature:      clientSignature,
		webhookUrl:           webhookUrl,
	}, nil
}

func newTrustlessRequestFromHttpContext(r *http.Request) (*trustlessRequest, error) {
	httpRequestBody := struct {
		Intent    string  `json:"intent"`
		Message   string  `json:"message"`
		Signature string  `json:"signature"`
		Webhook   *string `json:"webhook"`
	}{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &httpRequestBody)
	if err != nil {
		return nil, err
	}

	rendezvousKey, err := common.NewAccountFromPublicKeyString(httpRequestBody.Intent)
	if err != nil {
		return nil, errors.New("intent is not a public key")
	}

	var signature solana.Signature
	decodedSignature, err := base58.Decode(httpRequestBody.Signature)
	if err != nil || len(decodedSignature) != len(signature) {
		return nil, errors.New("signature is invalid")
	}
	copy(signature[:], decodedSignature)

	var requestToReceiveBill messagingpb.RequestToReceiveBill
	messageBytes, err := base64.RawURLEncoding.DecodeString(httpRequestBody.Message)
	if err != nil {
		return nil, errors.New("message not valid base64")
	}
	err = proto.Unmarshal(messageBytes, &requestToReceiveBill)
	if err != nil {
		return nil, errors.New("message bytes is not a RequestToReceiveBill")
	}

	// Note: Validation occurs at the messaging service
	protoMessage := &messagingpb.Message{
		Kind: &messagingpb.Message_RequestToReceiveBill{
			RequestToReceiveBill: &requestToReceiveBill,
		},
	}

	if httpRequestBody.Webhook != nil {
		err = netutil.ValidateHttpUrl(*httpRequestBody.Webhook, true, false)
		if err != nil {
			return nil, err
		}
	}

	return newTrustlessRequest(
		protoMessage,
		rendezvousKey,
		signature,
		httpRequestBody.Webhook,
	)
}

func (r *trustlessRequest) GetPublicRendezvousKey() *common.Account {
	return r.publicRendezvousKey
}

func (r *trustlessRequest) GetClientSignature() solana.Signature {
	return r.clientSignature
}

func (r *trustlessRequest) ToProtoMessage() *messagingpb.Message {
	return r.originalProtoMessage
}

func newGetLoggedInUserIdRequestFromHttpContext(r *http.Request) (*userpb.GetLoginForThirdPartyAppRequest, error) {
	httpRequestBody := struct {
		Intent    string `json:"intent"`
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &httpRequestBody)
	if err != nil {
		return nil, err
	}

	intentId, err := common.NewAccountFromPublicKeyString(httpRequestBody.Intent)
	if err != nil {
		return nil, errors.New("intent is not a public key")
	}

	var protoRequest userpb.GetLoginForThirdPartyAppRequest
	decoded, err := base64.RawURLEncoding.DecodeString(httpRequestBody.Message)
	if err != nil {
		return nil, errors.New("message not valid base64")
	}
	err = proto.Unmarshal(decoded, &protoRequest)
	if err != nil {
		return nil, errors.New("message bytes is not a GetLoginForThirdPartyAppRequest")
	} else if err := protoRequest.Validate(); err != nil {
		return nil, errors.Wrap(err, "message failed proto validation")
	}

	var signature solana.Signature
	decodedSignature, err := base58.Decode(httpRequestBody.Signature)
	if err != nil || len(decodedSignature) != len(signature) {
		return nil, errors.New("signature is invalid")
	}
	copy(signature[:], decodedSignature)

	protoRequest.IntentId = &commonpb.IntentId{Value: intentId.ToProto().Value}
	protoRequest.Signature = &commonpb.Signature{Value: decodedSignature}
	return &protoRequest, nil
}
