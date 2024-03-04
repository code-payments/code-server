package chat

import (
	"time"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
)

// ToBlockchainMessage converts a raw blockchain message into a protobuf chat message for the merchant domain's chat.
func ToBlockchainMessage(
	signature string,
	sender *common.Account,
	blockchainMessage *thirdparty.NaclBoxBlockchainMessage,
	ts time.Time,
) (*chatpb.ChatMessage, error) {
	content := createEncryptedContent(sender, blockchainMessage)
	return newProtoChatMessage(signature, content, ts)
}

// createEncryptedContent creates the encrypted content for the chat message from the blockchain message.
func createEncryptedContent(sender *common.Account, blockchainMessage *thirdparty.NaclBoxBlockchainMessage) []*chatpb.Content {
	return []*chatpb.Content{
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
}
