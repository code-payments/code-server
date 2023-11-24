package thirdparty

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"

	"github.com/jdgcs/ed25519/extra25519"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/vence722/base122-go"
	"golang.org/x/crypto/nacl/box"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/code/common"
)

const (
	// Shared buffer for domain, encrypted message, and other dynamically sized values in encoding scheme
	//
	// Note: May be different per type and version of blockchain message in the future
	// Note: Assumes up to two additional token transfer instructions in the transactions for paying fees and user
	maxDynamicContentSize = 650
)

type BlockchainMessageType uint8

const (
	NaclBoxBlockchainMessage BlockchainMessageType = iota
)

const naclBoxNonceLength = 24

type naclBoxNonce [naclBoxNonceLength]byte

// BlockchainMessage is an encrypted message, plus associated metadata, sent
// over the blockchain.
type BlockchainMessage struct {
	Type             BlockchainMessageType
	Version          uint8
	Flags            uint32
	SenderDomain     string // Subdomains are allowed, but unused. There could be something there as a feature.
	ReceiverAccount  *common.Account
	Nonce            []byte
	EncryptedMessage []byte
}

// NewNaclBoxBlockchainMessage returns a new BlockchainMessage using a NCAL box
// where the shared key is derived using ECDH.
func NewNaclBoxBlockchainMessage(
	senderDomain string,
	plaintextMessage string,
	sender *common.Account,
	receiver *common.Account,
) (*BlockchainMessage, error) {
	if err := netutil.ValidateDomainName(senderDomain); err != nil {
		return nil, errors.Wrap(err, "domain is invalid")
	}

	plaintextMessage = strings.TrimSpace(plaintextMessage)
	if len(plaintextMessage) == 0 {
		return nil, errors.New("plaintext message is empty")
	}

	if sender.PrivateKey() == nil {
		return nil, errors.New("sender account private key unavailable")
	}

	encryptedMessage, nonce := encryptMessageUsingNaclBox(sender, receiver, plaintextMessage)

	if len(encryptedMessage)+len(senderDomain) > maxDynamicContentSize {
		return nil, errors.New("encrypted message length exceeds limit")
	}

	return &BlockchainMessage{
		Type:             NaclBoxBlockchainMessage,
		Version:          0,
		Flags:            0,
		SenderDomain:     senderDomain,
		ReceiverAccount:  receiver,
		Nonce:            nonce[:],
		EncryptedMessage: encryptedMessage,
	}, nil
}

// Encode encodes the BlockchainMessage into a format compatible with the Solana
// memo instruction.
func (m *BlockchainMessage) Encode() ([]byte, error) {
	var buffer []byte

	buffer = append(buffer, uint8(m.Type))

	buffer = append(buffer, m.Version)

	var encodedFlags [4]byte
	binary.LittleEndian.PutUint32(encodedFlags[:], m.Flags)
	buffer = append(buffer, encodedFlags[:]...)

	buffer = append(buffer, uint8(len(m.SenderDomain)))
	buffer = append(buffer, []byte(m.SenderDomain)...)

	buffer = append(buffer, m.ReceiverAccount.PublicKey().ToBytes()...)

	buffer = append(buffer, m.Nonce...)
	buffer = append(buffer, []byte(m.EncryptedMessage)...)

	// Because memo requires UTF-8, and this is more space efficient than base64
	return base122.Encode(buffer)
}

// ToProto creates the proto representation of a BlockchainMessage
func (m *BlockchainMessage) ToProto(
	sender *common.Account,
	signature string,
	ts time.Time,
) (*chatpb.ChatMessage, error) {
	messageId, err := base58.Decode(signature)
	if err != nil {
		return nil, err
	}

	msg := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: messageId,
		},
		Ts: timestamppb.New(ts),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    sender.ToProto(),
						Nonce:            m.Nonce,
						EncryptedPayload: m.EncryptedMessage,
					},
				},
			},
		},
	}

	if err := msg.Content[0].Validate(); err != nil {
		return nil, errors.Wrap(err, "unexpectedly constructed an invalid proto message")
	}
	return msg, nil
}

// DecodeBlockchainMessages attempts to decode a byte payload into a BlockchainMessage
func DecodeBlockchainMessage(payload []byte) (*BlockchainMessage, error) {
	errInvalidPayload := errors.New("invalid payload")

	buffer, err := base122.Decode(payload)
	if err != nil {
		return nil, errors.Wrap(err, errInvalidPayload.Error())
	}

	if len(buffer) == 0 {
		return nil, errInvalidPayload
	}

	messageType := BlockchainMessageType(buffer[0])
	if messageType != NaclBoxBlockchainMessage {
		return nil, errors.Errorf("message type %d is not supported", messageType)
	}

	// 1  + (messageType)
	// 1  + (version)
	// 4  + (flags)
	// 1  + (domainNameSize)
	// 32 + (receiverAccount)
	// 24 + (nonce)
	// 1    (message)
	const minMessageSize = 64

	if len(buffer) < minMessageSize {
		return nil, errInvalidPayload
	}

	version := buffer[1]
	if version != 0 {
		return nil, errors.Errorf("message type %d version %d is not supported", version, messageType)
	}

	flags := binary.LittleEndian.Uint32(buffer[2:6])

	domainNameSize := int(buffer[6])

	if len(buffer) < minMessageSize+domainNameSize {
		return nil, errInvalidPayload
	}

	senderDomain := string(buffer[7 : 7+domainNameSize])
	if err := netutil.ValidateDomainName(senderDomain); err != nil {
		return nil, err
	}
	offset := 7 + domainNameSize

	receiverAccount, err := common.NewAccountFromPublicKeyBytes(buffer[offset : offset+ed25519.PublicKeySize])
	if err != nil {
		return nil, errors.Wrap(err, "invalid receiver account")
	}
	offset += ed25519.PublicKeySize

	var nonce naclBoxNonce
	copy(nonce[:], buffer[offset:offset+len(nonce)])
	offset += len(nonce)

	encryptedMessage := buffer[offset:]
	if len(encryptedMessage)+len(senderDomain) > maxDynamicContentSize {
		return nil, errors.New("encrypted message length exceeds limit")
	}

	return &BlockchainMessage{
		Type:             messageType,
		Version:          version,
		Flags:            flags,
		SenderDomain:     senderDomain,
		ReceiverAccount:  receiverAccount,
		Nonce:            nonce[:],
		EncryptedMessage: encryptedMessage,
	}, nil
}

func encryptMessageUsingNaclBox(sender, receiver *common.Account, plaintextMessage string) ([]byte, naclBoxNonce) {
	var nonce [24]byte
	rand.Read(nonce[:])

	var curve25519PrivateKey [32]byte
	var ed25519PrivateKey [64]byte
	copy(ed25519PrivateKey[:], sender.PrivateKey().ToBytes())
	extra25519.PrivateKeyToCurve25519(&curve25519PrivateKey, &ed25519PrivateKey)

	var curve25519PublicKey [32]byte
	var ed25519PublicKey [32]byte
	copy(ed25519PublicKey[:], receiver.PublicKey().ToBytes())
	extra25519.PublicKeyToCurve25519(&curve25519PublicKey, &ed25519PublicKey)

	encrypted := box.Seal(nil, []byte(plaintextMessage), &nonce, &curve25519PublicKey, &curve25519PrivateKey)

	return encrypted, nonce
}

func decryptMessageUsingNaclBox(receiver, sender *common.Account, encryptedPayload []byte, nonce [24]byte) (string, error) {
	var curve25519PrivateKey [32]byte
	var ed25519PrivateKey [64]byte
	copy(ed25519PrivateKey[:], receiver.PrivateKey().ToBytes())
	extra25519.PrivateKeyToCurve25519(&curve25519PrivateKey, &ed25519PrivateKey)

	var curve25519PublicKey [32]byte
	var ed25519PublicKey [32]byte
	copy(ed25519PublicKey[:], sender.PublicKey().ToBytes())
	extra25519.PublicKeyToCurve25519(&curve25519PublicKey, &ed25519PublicKey)

	message, opened := box.Open(nil, encryptedPayload, &nonce, &curve25519PublicKey, &curve25519PrivateKey)
	if !opened {
		return "", errors.New("failed decrypting payload")
	}
	return string(message), nil
}
