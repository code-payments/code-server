package chat_v1

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/pointer"
)

type ChatType uint8

const (
	ChatTypeUnknown  ChatType = iota
	ChatTypeInternal          // todo: better name, or split into the various buckets (eg. Code Team vs Cash Transactions)
	ChatTypeExternalApp
)

type ChatId [32]byte

type Chat struct {
	Id uint64

	ChatId     ChatId
	ChatType   ChatType
	ChatTitle  string // The message sender
	IsVerified bool

	CodeUser string // Always a receiver of messages

	ReadPointer    *string
	IsMuted        bool
	IsUnsubscribed bool

	CreatedAt time.Time
}

type Message struct {
	Id uint64

	ChatId ChatId

	MessageId string
	Data      []byte

	IsSilent      bool
	ContentLength uint8

	Timestamp time.Time
}

func GetChatId(sender, receiver string, isVerified bool) ChatId {
	combined := []byte(fmt.Sprintf("%s:%s:%v", sender, receiver, isVerified))
	if strings.Compare(sender, receiver) > 0 {
		combined = []byte(fmt.Sprintf("%s:%s:%v", receiver, sender, isVerified))
	}
	return sha256.Sum256(combined)
}

func (c ChatId) ToProto() *chatpb.ChatId {
	return &chatpb.ChatId{
		Value: c[:],
	}
}

func ChatIdFromProto(proto *chatpb.ChatId) ChatId {
	var chatId ChatId
	copy(chatId[:], proto.Value)
	return chatId
}

func (c ChatId) String() string {
	return hex.EncodeToString(c[:])
}

func (r *Chat) Validate() error {
	expectedChatId := GetChatId(r.CodeUser, r.ChatTitle, r.IsVerified)
	if !bytes.Equal(r.ChatId[:], expectedChatId[:]) {
		return errors.New("chat id is invalid")
	}

	if r.ChatType == ChatTypeUnknown {
		return errors.New("chat type is required")
	}

	if len(r.ChatTitle) == 0 {
		return errors.New("chat title is required")
	}

	if len(r.CodeUser) == 0 {
		return errors.New("code user is required")
	}

	if r.ReadPointer != nil && len(*r.ReadPointer) == 0 {
		return errors.New("read pointer is required when set")
	}

	return nil
}

func (r *Chat) Clone() Chat {
	var chatIdCopy ChatId
	copy(chatIdCopy[:], r.ChatId[:])

	return Chat{
		Id: r.Id,

		ChatId:     chatIdCopy,
		ChatType:   r.ChatType,
		ChatTitle:  r.ChatTitle,
		IsVerified: r.IsVerified,

		CodeUser: r.CodeUser,

		ReadPointer: pointer.StringCopy(r.ReadPointer),

		IsMuted:        r.IsMuted,
		IsUnsubscribed: r.IsUnsubscribed,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Chat) CopyTo(dst *Chat) {
	dst.Id = r.Id

	copy(dst.ChatId[:], r.ChatId[:])
	dst.ChatType = r.ChatType
	dst.ChatTitle = r.ChatTitle
	dst.IsVerified = r.IsVerified

	dst.CodeUser = r.CodeUser

	dst.ReadPointer = pointer.StringCopy(r.ReadPointer)

	dst.IsMuted = r.IsMuted
	dst.IsUnsubscribed = r.IsUnsubscribed

	dst.CreatedAt = r.CreatedAt
}

func (r *Message) Validate() error {
	if len(r.Data) == 0 {
		return errors.New("data is required")
	}

	if len(r.MessageId) == 0 {
		return errors.New("message id is required")
	}

	if r.ContentLength <= 0 {
		return errors.New("content length must be positive")
	}

	if r.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}

	return nil
}

func (r *Message) Clone() Message {
	var chatIdCopy ChatId
	copy(chatIdCopy[:], r.ChatId[:])

	dataCopy := make([]byte, len(r.Data))
	copy(dataCopy, r.Data)

	return Message{
		Id: r.Id,

		ChatId: chatIdCopy,

		MessageId: r.MessageId,
		Data:      dataCopy,

		IsSilent:      r.IsSilent,
		ContentLength: r.ContentLength,

		Timestamp: r.Timestamp,
	}
}

func (r *Message) CopyTo(dst *Message) {
	dst.Id = r.Id

	dst.ChatId = r.ChatId

	dst.MessageId = r.MessageId
	dst.Data = make([]byte, len(r.Data))
	copy(dst.Data, r.Data)

	dst.IsSilent = r.IsSilent
	dst.ContentLength = r.ContentLength

	dst.Timestamp = r.Timestamp
}
