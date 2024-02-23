package chat

import (
	"time"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
)

// ToBlockchainMessage takes a raw blockchain message and turns it into a protobuf
// chat message that can be injected into the merchant domain's chat.
func ToBlockchainMessage(
	signature string,
	sender *common.Account,
	blockchainMessage *thirdparty.NaclBoxBlockchainMessage,
	ts time.Time,
) (*chatpb.ChatMessage, error) {
	content := []*chatpb.Content{
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
	return newProtoChatMessage(signature, content, ts)
}
