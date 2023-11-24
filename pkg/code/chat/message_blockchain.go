package chat

import (
	"time"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
)

// ToBlockchainMessage takes a raw blockchain message and turns it into a protobuf
// chat message that can be injected into the merchant domain's chat.
func ToBlockchainMessage(
	signature string,
	sender *common.Account,
	blockchainMessage *thirdparty.BlockchainMessage,
	ts time.Time,
) (*chatpb.ChatMessage, error) {
	var content []*chatpb.Content
	switch blockchainMessage.Type {
	case thirdparty.NaclBoxBlockchainMessage:
		content = []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    sender.ToProto(),
						Nonce:            blockchainMessage.Nonce,
						EncryptedPayload: blockchainMessage.EncryptedMessage,
					},
				},
			},
		}
	default:
		return nil, errors.Errorf("%d blockchain message type not supported", blockchainMessage.Type)
	}

	return newProtoChatMessage(signature, content, ts)
}
