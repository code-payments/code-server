package chat_v2

import (
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/pointer"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
)

type ChatType uint8

const (
	ChatTypeUnknown ChatType = iota
	ChatTypeNotification
	ChatTypeTwoWay
	// ChatTypeGroup
)

type ReferenceType uint8

const (
	ReferenceTypeUnknown ReferenceType = iota
	ReferenceTypeIntent
	ReferenceTypeSignature
)

type PointerType uint8

const (
	PointerTypeUnknown PointerType = iota
	PointerTypeSent
	PointerTypeDelivered
	PointerTypeRead
)

type Platform uint8

const (
	PlatformUnknown Platform = iota
	PlatformCode
	PlatformTwitter
)

type ChatRecord struct {
	Id     int64
	ChatId ChatId

	ChatType ChatType

	// Presence determined by ChatType:
	//  * Notification: Present, and may be a localization key
	//  * Two Way: Not present and generated dynamically based on chat members
	//  * Group: Present, and will not be a localization key
	ChatTitle *string

	IsVerified bool

	CreatedAt time.Time
}

type MemberRecord struct {
	Id       int64
	ChatId   ChatId
	MemberId MemberId

	Platform   Platform
	PlatformId string

	DeliveryPointer *MessageId
	ReadPointer     *MessageId

	IsMuted        bool
	IsUnsubscribed bool

	JoinedAt time.Time
}

type MessageRecord struct {
	Id        int64
	ChatId    ChatId
	MessageId MessageId

	// Not present for notification-style chats
	Sender *MemberId

	Data []byte

	ReferenceType *ReferenceType
	Reference     *string

	IsSilent bool

	// Note: No timestamp field, since it's encoded in MessageId
}

type MessagesById []*MessageRecord

func (a MessagesById) Len() int      { return len(a) }
func (a MessagesById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MessagesById) Less(i, j int) bool {
	return a[i].MessageId.Before(a[j].MessageId)
}

// GetChatIdFromProto gets a chat ID from the protobuf variant
func GetPointerTypeFromProto(proto chatpb.Pointer_Kind) PointerType {
	switch proto {
	case chatpb.Pointer_SENT:
		return PointerTypeSent
	case chatpb.Pointer_DELIVERED:
		return PointerTypeDelivered
	case chatpb.Pointer_READ:
		return PointerTypeRead
	default:
		return PointerTypeUnknown
	}
}

// ToProto returns the proto representation of the pointer type
func (p PointerType) ToProto() chatpb.Pointer_Kind {
	switch p {
	case PointerTypeSent:
		return chatpb.Pointer_SENT
	case PointerTypeDelivered:
		return chatpb.Pointer_DELIVERED
	case PointerTypeRead:
		return chatpb.Pointer_READ
	default:
		return chatpb.Pointer_UNKNOWN
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

// Validate validates a chat Record
func (r *ChatRecord) Validate() error {
	if err := r.ChatId.Validate(); err != nil {
		return errors.Wrap(err, "invalid chat id")
	}

	switch r.ChatType {
	case ChatTypeNotification:
		if r.ChatTitle == nil || len(*r.ChatTitle) == 0 {
			return errors.New("chat title is required for notification chats")
		}
	case ChatTypeTwoWay:
		if r.ChatTitle != nil {
			return errors.New("chat title cannot be set for two way chats")
		}
	default:
		return errors.Errorf("invalid chat type: %d", r.ChatType)
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation timestamp is required")
	}

	return nil
}

// Clone clones a chat record
func (r *ChatRecord) Clone() ChatRecord {
	return ChatRecord{
		Id:     r.Id,
		ChatId: r.ChatId,

		ChatType: r.ChatType,

		ChatTitle: pointer.StringCopy(r.ChatTitle),

		IsVerified: r.IsVerified,

		CreatedAt: r.CreatedAt,
	}
}

// CopyTo copies a chat record to the provided destination
func (r *ChatRecord) CopyTo(dst *ChatRecord) {
	dst.Id = r.Id
	dst.ChatId = r.ChatId

	dst.ChatType = r.ChatType

	dst.ChatTitle = pointer.StringCopy(r.ChatTitle)

	dst.IsVerified = r.IsVerified

	dst.CreatedAt = r.CreatedAt
}

// Validate validates a member Record
func (r *MemberRecord) Validate() error {
	if err := r.ChatId.Validate(); err != nil {
		return errors.Wrap(err, "invalid chat id")
	}

	if err := r.MemberId.Validate(); err != nil {
		return errors.Wrap(err, "invalid member id")
	}

	if len(r.PlatformId) == 0 {
		return errors.New("platform id is required")
	}

	switch r.Platform {
	case PlatformCode:
		decoded, err := base58.Decode(r.PlatformId)
		if err != nil {
			return errors.Wrap(err, "invalid base58 plaftorm id")
		}

		if len(decoded) != 32 {
			return errors.Wrap(err, "platform id is not a 32 byte buffer")
		}
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

		Platform:   r.Platform,
		PlatformId: r.PlatformId,

		DeliveryPointer: deliveryPointerCopy,
		ReadPointer:     readPointerCopy,

		IsMuted:        r.IsMuted,
		IsUnsubscribed: r.IsUnsubscribed,

		JoinedAt: r.JoinedAt,
	}
}

// CopyTo copies a member record to the provided destination
func (r *MemberRecord) CopyTo(dst *MemberRecord) {
	dst.Id = r.Id
	dst.ChatId = r.ChatId
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
	dst.IsUnsubscribed = r.IsUnsubscribed

	dst.JoinedAt = r.JoinedAt
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

	if len(r.Data) == 0 {
		return errors.New("message data is required")
	}

	if r.Reference == nil && r.ReferenceType != nil {
		return errors.New("reference is required when reference type is provided")
	}

	if r.Reference != nil && r.ReferenceType == nil {
		return errors.New("reference cannot be set when reference type is missing")
	}

	if r.ReferenceType != nil {
		switch *r.ReferenceType {
		case ReferenceTypeIntent:
			decoded, err := base58.Decode(*r.Reference)
			if err != nil {
				return errors.Wrap(err, "invalid base58 intent id reference")
			}

			if len(decoded) != 32 {
				return errors.Wrap(err, "reference is not a 32 byte buffer")
			}
		case ReferenceTypeSignature:
			decoded, err := base58.Decode(*r.Reference)
			if err != nil {
				return errors.Wrap(err, "invalid base58 signature reference")
			}

			if len(decoded) != 64 {
				return errors.Wrap(err, "reference is not a 64 byte buffer")
			}
		default:
			return errors.Errorf("invalid reference type: %d", *r.ReferenceType)
		}
	}

	return nil
}

// Clone clones a message record
func (r *MessageRecord) Clone() MessageRecord {
	var senderCopy *MemberId
	if r.Sender != nil {
		cloned := r.Sender.Clone()
		senderCopy = &cloned
	}

	dataCopy := make([]byte, len(r.Data))
	copy(dataCopy, r.Data)

	var referenceTypeCopy *ReferenceType
	if r.ReferenceType != nil {
		cloned := *r.ReferenceType
		referenceTypeCopy = &cloned
	}

	return MessageRecord{
		Id:        r.Id,
		ChatId:    r.ChatId,
		MessageId: r.MessageId,

		Sender: senderCopy,

		Data: dataCopy,

		ReferenceType: referenceTypeCopy,
		Reference:     pointer.StringCopy(r.Reference),

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

	dataCopy := make([]byte, len(r.Data))
	copy(dataCopy, r.Data)
	dst.Data = dataCopy

	if r.ReferenceType != nil {
		cloned := *r.ReferenceType
		dst.ReferenceType = &cloned
	}
	dst.Reference = pointer.StringCopy(r.Reference)

	dst.IsSilent = r.IsSilent
}

// GetTimestamp gets the timestamp for a message record
func (r *MessageRecord) GetTimestamp() (time.Time, error) {
	return r.MessageId.GetTimestamp()
}
