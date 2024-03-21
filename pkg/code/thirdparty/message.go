package thirdparty

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdgcs/ed25519/extra25519"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/vence722/base122-go"
	"golang.org/x/crypto/nacl/box"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/netutil"
)

const (
	// Shared buffer for domain, encrypted message, and other dynamically sized values in encoding scheme
	//
	// Note: May be different per type and version of blockchain message in the future
	// Note: Assumes up to two additional token transfer instructions in the transactions for paying fees and user
	maxNaclBoxDynamicContentSize = 650
)

type BlockchainMessageType uint8

const (
	NaclBoxType BlockchainMessageType = iota
	FiatOnrampPurchase
)

const naclBoxNonceLength = 24

type naclBoxNonce [naclBoxNonceLength]byte

// NaclBoxBlockchainMessage is an encrypted message, plus associated metadata, sent
// over the blockchain.
type NaclBoxBlockchainMessage struct {
	Version          uint8
	Flags            uint32
	SenderDomain     string // Subdomains are allowed, but unused. There could be something there as a feature.
	ReceiverAccount  *common.Account
	Nonce            []byte
	EncryptedMessage []byte
}

// NewNaclBoxBlockchainMessage returns a new BlockchainMessage using a NACL box
// where the shared key is derived using ECDH.
func NewNaclBoxBlockchainMessage(
	senderDomain string,
	plaintextMessage string,
	sender *common.Account,
	receiver *common.Account,
) (*NaclBoxBlockchainMessage, error) {
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

	if len(encryptedMessage)+len(senderDomain) > maxNaclBoxDynamicContentSize {
		return nil, errors.New("encrypted message length exceeds limit")
	}

	return &NaclBoxBlockchainMessage{
		Version:          0,
		Flags:            0,
		SenderDomain:     senderDomain,
		ReceiverAccount:  receiver,
		Nonce:            nonce[:],
		EncryptedMessage: encryptedMessage,
	}, nil
}

// Encode encodes the NaclBoxBlockchainMessage into a format compatible with the Solana
// memo instruction.
func (m *NaclBoxBlockchainMessage) Encode() ([]byte, error) {
	var buffer []byte

	buffer = append(buffer, uint8(NaclBoxType))

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

// ToProto creates the proto representation of a NaclBoxBlockchainMessage
func (m *NaclBoxBlockchainMessage) ToProto(
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

// DecodeNaclBoxBlockchainMessage attempts to decode a byte payload into a NaclBoxBlockchainMessage
func DecodeNaclBoxBlockchainMessage(payload []byte) (*NaclBoxBlockchainMessage, error) {
	errInvalidPayload := errors.New("invalid payload")

	buffer, err := base122.Decode(payload)
	if err != nil {
		return nil, errors.Wrap(err, errInvalidPayload.Error())
	}

	if len(buffer) == 0 {
		return nil, errInvalidPayload
	}

	messageType := BlockchainMessageType(buffer[0])
	if messageType != NaclBoxType {
		return nil, errors.Errorf("expected message type %d", NaclBoxType)
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
		return nil, errors.Errorf("version %d is not supported", version)
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
	if len(encryptedMessage)+len(senderDomain) > maxNaclBoxDynamicContentSize {
		return nil, errors.New("encrypted message length exceeds limit")
	}

	return &NaclBoxBlockchainMessage{
		Version:          version,
		Flags:            flags,
		SenderDomain:     senderDomain,
		ReceiverAccount:  receiverAccount,
		Nonce:            nonce[:],
		EncryptedMessage: encryptedMessage,
	}, nil
}

func encryptMessageUsingNaclBox(sender, receiver *common.Account, plaintextMessage string) ([]byte, naclBoxNonce) {
	var nonce naclBoxNonce
	rand.Read(nonce[:])
	return encryptMessageUsingNaclBoxWithProvidedNonce(sender, receiver, plaintextMessage, nonce), nonce
}

// Nonce should always be random. Use encryptMessageUsingNaclBox, unless testing
// hardcoded values.
func encryptMessageUsingNaclBoxWithProvidedNonce(sender, receiver *common.Account, plaintextMessage string, nonce naclBoxNonce) []byte {
	curve25519PrivateKey := ed25519ToCurve25519PrivateKey(sender)
	curve25519PublicKey := ed25519ToCurve25519PublicKey(receiver)

	encrypted := box.Seal(nil, []byte(plaintextMessage), (*[naclBoxNonceLength]byte)(&nonce), &curve25519PublicKey, &curve25519PrivateKey)

	return encrypted
}

func decryptMessageUsingNaclBox(sender, receiver *common.Account, encryptedPayload []byte, nonce naclBoxNonce) (string, error) {
	curve25519PrivateKey := ed25519ToCurve25519PrivateKey(receiver)
	curve25519PublicKey := ed25519ToCurve25519PublicKey(sender)

	message, opened := box.Open(nil, encryptedPayload, (*[naclBoxNonceLength]byte)(&nonce), &curve25519PublicKey, &curve25519PrivateKey)
	if !opened {
		return "", errors.New("failed decrypting payload")
	}
	return string(message), nil
}

func ed25519ToCurve25519PublicKey(account *common.Account) [32]byte {
	var curve25519PublicKey [32]byte
	var ed25519PublicKey [32]byte
	copy(ed25519PublicKey[:], account.PublicKey().ToBytes())
	extra25519.PublicKeyToCurve25519(&curve25519PublicKey, &ed25519PublicKey)
	return curve25519PublicKey
}

func ed25519ToCurve25519PrivateKey(account *common.Account) [32]byte {
	var curve25519PrivateKey [32]byte
	var ed25519PrivateKey [64]byte
	copy(ed25519PrivateKey[:], account.PrivateKey().ToBytes())
	extra25519.PrivateKeyToCurve25519(&curve25519PrivateKey, &ed25519PrivateKey)
	return curve25519PrivateKey
}

type FiatOnrampPurchaseMessage struct {
	Version uint8
	Flags   uint32
	Nonce   uuid.UUID
}

// NewFiatOnrampPurchaseMessage returns a new BlockchainMessage used to indicate
// fulfillmenet of a fiat purchase by an onramp provider.
func NewFiatOnrampPurchaseMessage(nonce uuid.UUID) (*FiatOnrampPurchaseMessage, error) {
	return &FiatOnrampPurchaseMessage{
		Version: 0,
		Flags:   0,
		Nonce:   nonce,
	}, nil
}

// Encode encodes the FiatOnrampPurchaseMessage into a format compatible with the Solana
// memo instruction.
func (m *FiatOnrampPurchaseMessage) Encode() ([]byte, error) {
	var buffer []byte

	buffer = append(buffer, uint8(FiatOnrampPurchase))

	buffer = append(buffer, m.Version)

	var encodedFlags [4]byte
	binary.LittleEndian.PutUint32(encodedFlags[:], m.Flags)
	buffer = append(buffer, encodedFlags[:]...)

	buffer = append(buffer, m.Nonce[:]...)

	return []byte(base64.StdEncoding.EncodeToString(buffer)), nil
}

// DecodeFiatOnrampPurchaseMessage attempts to decode a byte payload into a FiatOnrampPurchaseMessage
func DecodeFiatOnrampPurchaseMessage(payload []byte) (*FiatOnrampPurchaseMessage, error) {
	errInvalidPayload := errors.New("invalid payload")

	buffer, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return nil, errors.Wrap(err, errInvalidPayload.Error())
	}

	if len(buffer) == 0 {
		return nil, errInvalidPayload
	}

	messageType := BlockchainMessageType(buffer[0])
	if messageType != FiatOnrampPurchase {
		return nil, errors.Errorf("expected message type %d", FiatOnrampPurchase)
	}

	// 1  + (messageType)
	// 1  + (version)
	// 4  + (flags)
	// 16  + (nonce)
	const messageSize = 22

	if len(buffer) != messageSize {
		return nil, errInvalidPayload
	}

	version := buffer[1]
	if version != 0 {
		return nil, errors.Errorf("version %d is not supported", version)
	}

	flags := binary.LittleEndian.Uint32(buffer[2:6])

	nonce, err := uuid.FromBytes(buffer[6:])
	if err != nil {
		return nil, errors.Wrap(err, "nonce is invalid")
	}

	return &FiatOnrampPurchaseMessage{
		Version: version,
		Flags:   flags,
		Nonce:   nonce,
	}, nil
}
