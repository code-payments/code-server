package chat_v2

import (
	"bytes"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
)

type ChatId [32]byte

// GetChatIdFromProto gets a chat ID from the protobuf variant
func GetChatIdFromProto(proto *chatpb.ChatId) (ChatId, error) {
	if err := proto.Validate(); err != nil {
		return ChatId{}, errors.Wrap(err, "proto validation failed")
	}

	var typed ChatId
	copy(typed[:], proto.Value)

	if err := typed.Validate(); err != nil {
		return ChatId{}, errors.Wrap(err, "invalid chat id")
	}

	return typed, nil
}

// Validate validates a chat ID
func (c ChatId) Validate() error {
	return nil
}

// String returns the string representation of a ChatId
func (c ChatId) String() string {
	return hex.EncodeToString(c[:])
}

// Random UUIDv4 ID for chat members
type MemberId uuid.UUID

// GetMemberIdFromProto gets a member ID from the protobuf variant
func GetMemberIdFromProto(proto *chatpb.ChatMemberId) (MemberId, error) {
	if err := proto.Validate(); err != nil {
		return MemberId{}, errors.Wrap(err, "proto validation failed")
	}

	var typed MemberId
	copy(typed[:], proto.Value)

	if err := typed.Validate(); err != nil {
		return MemberId{}, errors.Wrap(err, "invalid member id")
	}

	return typed, nil
}

// GenerateMemberId generates a new random chat member ID
func GenerateMemberId() MemberId {
	return MemberId(uuid.New())
}

// Validate validates a chat member ID
func (m MemberId) Validate() error {
	casted := uuid.UUID(m)

	if casted.Version() != 4 {
		return errors.Errorf("invalid uuid version: %s", casted.Version().String())
	}

	return nil
}

// String returns the string representation of a MemberId
func (m MemberId) String() string {
	return uuid.UUID(m).String()
}

// Time-based UUIDv7 ID for chat messages
type MessageId uuid.UUID

// GenerateMessageId generates a UUIDv7 message ID using the current time
func GenerateMessageId() MessageId {
	return GenerateMessageIdAtTime(time.Now())
}

// GenerateMessageIdAtTime generates a UUIDv7 message ID using the provided timestamp
func GenerateMessageIdAtTime(ts time.Time) MessageId {
	// Convert timestamp to milliseconds since Unix epoch
	millis := ts.UnixNano() / int64(time.Millisecond)

	// Create a byte slice to hold the UUID
	var uuidBytes [16]byte

	// Populate the first 6 bytes with the timestamp (42 bits for timestamp)
	uuidBytes[0] = byte((millis >> 40) & 0xff)
	uuidBytes[1] = byte((millis >> 32) & 0xff)
	uuidBytes[2] = byte((millis >> 24) & 0xff)
	uuidBytes[3] = byte((millis >> 16) & 0xff)
	uuidBytes[4] = byte((millis >> 8) & 0xff)
	uuidBytes[5] = byte(millis & 0xff)

	// Set the version to 7 (UUIDv7)
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | (0x7 << 4)

	// Populate the remaining bytes with random values
	randomUUID := uuid.New()
	copy(uuidBytes[7:], randomUUID[7:])

	return MessageId(uuidBytes)
}

// GetMessageIdFromProto gets a message ID from the protobuf variant
func GetMessageIdFromProto(proto *chatpb.ChatMessageId) (MessageId, error) {
	if err := proto.Validate(); err != nil {
		return MessageId{}, errors.Wrap(err, "proto validation failed")
	}

	var typed MessageId
	copy(typed[:], proto.Value)

	if err := typed.Validate(); err != nil {
		return MessageId{}, errors.Wrap(err, "invalid message id")
	}

	return typed, nil
}

// GetTimestamp gets the encoded timestamp in the message ID
func (m MessageId) GetTimestamp() (time.Time, error) {
	if err := m.Validate(); err != nil {
		return time.Time{}, errors.Wrap(err, "invalid message id")
	}

	// Extract the first 6 bytes as the timestamp
	millis := (int64(m[0]) << 40) | (int64(m[1]) << 32) | (int64(m[2]) << 24) |
		(int64(m[3]) << 16) | (int64(m[4]) << 8) | int64(m[5])

	// Convert milliseconds since Unix epoch to time.Time
	timestamp := time.Unix(0, millis*int64(time.Millisecond))

	return timestamp, nil
}

// Equals returns whether two message IDs are equal
func (m MessageId) Equals(other MessageId) bool {
	return m.Compare(other) == 0
}

// Before returns whether the message ID is before the provided value
func (m MessageId) Before(other MessageId) bool {
	return m.Compare(other) < 0
}

// Before returns whether the message ID is after the provided value
func (m MessageId) After(other MessageId) bool {
	return m.Compare(other) > 0
}

// Compare returns the byte comparison of the message ID
func (m MessageId) Compare(other MessageId) int {
	return bytes.Compare(m[:], other[:])
}

// Validate validates a message ID
func (m MessageId) Validate() error {
	casted := uuid.UUID(m)

	if casted.Version() != 7 {
		return errors.Errorf("invalid uuid version: %s", casted.Version().String())
	}

	return nil
}

// String returns the string representation of a MessageId
func (m MessageId) String() string {
	return uuid.UUID(m).String()
}
