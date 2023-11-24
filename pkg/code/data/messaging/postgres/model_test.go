package postgres

import (
	"crypto/ed25519"
	"testing"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/code/data/messaging"
)

func TestModelConversion(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	requestor, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &commonpb.SolanaAccountId{Value: pub}
	messageID := uuid.New()
	idBytes, _ := messageID.MarshalBinary()
	message := &messagingpb.Message{
		Id: &messagingpb.MessageId{
			Value: idBytes,
		},
		Kind: &messagingpb.Message_RequestToGrabBill{
			RequestToGrabBill: &messagingpb.RequestToGrabBill{
				RequestorAccount: &commonpb.SolanaAccountId{
					Value: requestor,
				},
			},
		},
	}
	messageBytes, err := proto.Marshal(message)
	require.NoError(t, err)

	record := &messaging.Record{
		Account:   base58.Encode(account.Value),
		MessageID: messageID,
		Message:   messageBytes,
	}

	model, err := toModel(record)
	require.NoError(t, err)
	assert.Equal(t, model.Account, base58.Encode(account.Value))
	assert.Equal(t, model.MessageID, messageID.String())
	assert.Equal(t, model.Message, messageBytes)

	actual, err := fromModel(model)
	require.NoError(t, err)
	assert.Equal(t, actual, record)
}
