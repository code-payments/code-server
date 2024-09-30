package chat_v2

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/pointer"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
)

type ChatType uint8

const (
	ChatTypeUnknown ChatType = iota
	ChatTypeTwoWay
	// ChatTypeGroup
)

// GetChatTypeFromProto gets a chat type from the protobuf variant
func GetChatTypeFromProto(proto chatpb.ChatType) ChatType {
	switch proto {
	case chatpb.ChatType_TWO_WAY:
		return ChatTypeTwoWay
	default:
		return ChatTypeUnknown
	}
}

// ToProto returns the proto representation of the chat type
func (c ChatType) ToProto() chatpb.ChatType {
	switch c {
	case ChatTypeTwoWay:
		return chatpb.ChatType_TWO_WAY
	default:
		return chatpb.ChatType_UNKNOWN_CHAT_TYPE
	}
}

// String returns the string representation of the chat type
func (c ChatType) String() string {
	switch c {
	case ChatTypeTwoWay:
		return "two-way"
	default:
		return "unknown"
	}
}

type PointerType uint8

const (
	PointerTypeUnknown PointerType = iota
	PointerTypeSent
	PointerTypeDelivered
	PointerTypeRead
)

// GetPointerTypeFromProto gets a chat ID from the protobuf variant
func GetPointerTypeFromProto(proto chatpb.PointerType) PointerType {
	switch proto {
	case chatpb.PointerType_SENT:
		return PointerTypeSent
	case chatpb.PointerType_DELIVERED:
		return PointerTypeDelivered
	case chatpb.PointerType_READ:
		return PointerTypeRead
	default:
		return PointerTypeUnknown
	}
}

// ToProto returns the proto representation of the pointer type
func (p PointerType) ToProto() chatpb.PointerType {
	switch p {
	case PointerTypeSent:
		return chatpb.PointerType_SENT
	case PointerTypeDelivered:
		return chatpb.PointerType_DELIVERED
	case PointerTypeRead:
		return chatpb.PointerType_READ
	default:
		return chatpb.PointerType_UNKNOWN_POINTER_TYPE
	}
}

// String returns the string representation of the pointer type
func (p PointerType) String() string {
	switch p {
	case PointerTypeSent:
		return "sent"
	case PointerTypeDelivered:
		return "delivered"
	case PointerTypeRead:
		return "read"
	default:
		return "unknown"
	}
}

type Platform uint8

const (
	PlatformUnknown Platform = iota
	PlatformTwitter
)

// GetPlatformFromProto returns the proto representation of the platform
func GetPlatformFromProto(proto chatpb.Platform) Platform {
	switch proto {
	case chatpb.Platform_TWITTER:
		return PlatformTwitter
	default:
		return PlatformUnknown
	}
}

// ToProto returns the proto representation of the platform
func (p Platform) ToProto() chatpb.Platform {
	switch p {
	case PlatformTwitter:
		return chatpb.Platform_TWITTER
	default:
		return chatpb.Platform_UNKNOWN_PLATFORM
	}
}

// String returns the string representation of the platform
func (p Platform) String() string {
	switch p {
	case PlatformTwitter:
		return "twitter"
	default:
		return "unknown"
	}
}

type MetadataRecord struct {
	Id        int64
	ChatId    ChatId
	ChatType  ChatType
	CreatedAt time.Time

	ChatTitle *string
}

// Validate validates a chat Record
func (r *MetadataRecord) Validate() error {
	if err := r.ChatId.Validate(); err != nil {
		return errors.Wrap(err, "invalid chat id")
	}

	switch r.ChatType {
	case ChatTypeTwoWay:
	default:
		return errors.Errorf("invalid chat type: %d", r.ChatType)
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation timestamp is required")
	}

	return nil
}

// Clone clones a chat record
func (r *MetadataRecord) Clone() MetadataRecord {
	return MetadataRecord{
		Id:        r.Id,
		ChatId:    r.ChatId,
		ChatType:  r.ChatType,
		CreatedAt: r.CreatedAt,

		ChatTitle: pointer.StringCopy(r.ChatTitle),
	}
}

// CopyTo copies a chat record to the provided destination
func (r *MetadataRecord) CopyTo(dst *MetadataRecord) {
	dst.Id = r.Id
	dst.ChatId = r.ChatId
	dst.ChatType = r.ChatType
	dst.CreatedAt = r.CreatedAt

	dst.ChatTitle = pointer.StringCopy(r.ChatTitle)
}

type MemberRecord struct {
	Id     int64
	ChatId ChatId

	// MemberId is derived from Owner (using account.ToMessagingAccount)
	//
	// It is stored to allow indexed lookups when only MemberId is available.
	// We must also store Owner so server can lookup proper push tokens.
	MemberId string

	// Owner is required to be able to send push notifications.
	//
	// Currently, it is _optional_, as we don't have a way to reverse lookup.
	// However, we _will_ want to make it mandatory.
	Owner string

	// Identity.
	//
	// Currently, assumes single.
	Platform   Platform
	PlatformId string

	DeliveryPointer *MessageId
	ReadPointer     *MessageId

	IsMuted  bool
	JoinedAt time.Time
}

// Validate validates a member Record
func (r *MemberRecord) Validate() error {
	if err := r.ChatId.Validate(); err != nil {
		return fmt.Errorf("invalid chat id: %w", err)
	}

	if len(r.MemberId) == 0 {
		return fmt.Errorf("missing member id")
	}

	if len(r.PlatformId) == 0 {
		return fmt.Errorf("missing platform id")
	}

	switch r.Platform {
	case PlatformTwitter:
		if len(r.PlatformId) > 15 {
			return errors.New("platform id must have at most 15 characters")
		}
	default:
		return errors.Errorf("invalid plaftorm: %d", r.Platform)
	}

	if r.DeliveryPointer != nil {
		if err := r.DeliveryPointer.Validate(); err != nil {
			return errors.Wrap(err, "invalid delivery pointer")
		}
	}

	if r.ReadPointer != nil {
		if err := r.ReadPointer.Validate(); err != nil {
			return errors.Wrap(err, "invalid read pointer")
		}
	}

	if r.JoinedAt.IsZero() {
		return errors.New("joined timestamp is required")
	}

	return nil
}

// Clone clones a member record
func (r *MemberRecord) Clone() MemberRecord {
	var deliveryPointerCopy *MessageId
	if r.DeliveryPointer != nil {
		cloned := r.DeliveryPointer.Clone()
		deliveryPointerCopy = &cloned
	}

	var readPointerCopy *MessageId
	if r.ReadPointer != nil {
		cloned := r.ReadPointer.Clone()
		readPointerCopy = &cloned
	}

	return MemberRecord{
		Id:       r.Id,
		ChatId:   r.ChatId,
		MemberId: r.MemberId,
		Owner:    r.Owner,

		Platform:   r.Platform,
		PlatformId: r.PlatformId,

		DeliveryPointer: deliveryPointerCopy,
		ReadPointer:     readPointerCopy,

		IsMuted:  r.IsMuted,
		JoinedAt: r.JoinedAt,
	}
}

// CopyTo copies a member record to the provided destination
func (r *MemberRecord) CopyTo(dst *MemberRecord) {
	dst.Id = r.Id
	dst.ChatId = r.ChatId
	dst.Owner = r.Owner
	dst.MemberId = r.MemberId

	dst.Platform = r.Platform
	dst.PlatformId = r.PlatformId

	if r.DeliveryPointer != nil {
		cloned := r.DeliveryPointer.Clone()
		dst.DeliveryPointer = &cloned
	}
	if r.ReadPointer != nil {
		cloned := r.ReadPointer.Clone()
		dst.ReadPointer = &cloned
	}

	dst.IsMuted = r.IsMuted
	dst.JoinedAt = r.JoinedAt
}

type MembersById []*MemberRecord

func (a MembersById) Len() int      { return len(a) }
func (a MembersById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MembersById) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

type MessageRecord struct {
	Id        int64
	ChatId    ChatId
	MessageId MessageId

	Sender *MemberId

	Payload []byte

	IsSilent bool

	// Note: No timestamp field, since it's encoded in MessageId
	// Note: Maybe a timestamp field, because it's maybe better?
}

// Validate validates a message Record
func (r *MessageRecord) Validate() error {
	if err := r.ChatId.Validate(); err != nil {
		return errors.Wrap(err, "invalid chat id")
	}

	if err := r.MessageId.Validate(); err != nil {
		return errors.Wrap(err, "invalid message id")
	}

	if r.Sender != nil {
		if err := r.Sender.Validate(); err != nil {
			return errors.Wrap(err, "invalid sender id")
		}
	}

	if len(r.Payload) == 0 {
		return errors.New("message payload is required")
	}

	return nil
}

type MessagesByMessageId []*MessageRecord

func (a MessagesByMessageId) Len() int      { return len(a) }
func (a MessagesByMessageId) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MessagesByMessageId) Less(i, j int) bool {
	return a[i].MessageId.Before(a[j].MessageId)
}

// Clone clones a message record
func (r *MessageRecord) Clone() MessageRecord {
	var senderCopy *MemberId
	if r.Sender != nil {
		cloned := r.Sender.Clone()
		senderCopy = &cloned
	}

	var payloadCopy []byte
	if len(r.Payload) > 0 {
		payloadCopy = make([]byte, len(r.Payload))
		copy(payloadCopy, r.Payload)
	}

	return MessageRecord{
		Id:        r.Id,
		ChatId:    r.ChatId,
		MessageId: r.MessageId,

		Sender: senderCopy,

		Payload: payloadCopy,

		IsSilent: r.IsSilent,
	}
}

// CopyTo copies a message record to the provided destination
func (r *MessageRecord) CopyTo(dst *MessageRecord) {
	dst.Id = r.Id
	dst.ChatId = r.ChatId
	dst.MessageId = r.MessageId

	if r.Sender != nil {
		cloned := r.Sender.Clone()
		dst.Sender = &cloned
	}

	payloadCopy := make([]byte, len(r.Payload))
	copy(payloadCopy, r.Payload)
	dst.Payload = payloadCopy

	dst.IsSilent = r.IsSilent
}

// GetTimestamp gets the timestamp for a message record
func (r *MessageRecord) GetTimestamp() (time.Time, error) {
	return r.MessageId.GetTimestamp()
}
