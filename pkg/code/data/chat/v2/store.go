package chat_v2

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrChatNotFound    = errors.New("chat not found")
	ErrMemberNotFound  = errors.New("chat member not found")
	ErrMessageNotFound = errors.New("chat message not found")
)

// todo: Define interface methods
type Store interface {
	// GetChatById gets a chat by its chat ID
	GetChatById(ctx context.Context, chatId ChatId) (*ChatRecord, error)

	// GetMemberById gets a chat member by the chat and member IDs
	GetMemberById(ctx context.Context, chatId ChatId, memberId MemberId) (*MemberRecord, error)

	// GetMessageById gets a chat message by the chat and message IDs
	GetMessageById(ctx context.Context, chatId ChatId, messageId MessageId) (*MessageRecord, error)

	// GetAllMessagesByChat gets all messages for a given chat
	//
	// Note: Cursor is a message ID
	GetAllMessagesByChat(ctx context.Context, chatId ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*MessageRecord, error)

	// PutMessage creates a new chat message
	PutMessage(ctx context.Context, record *MessageRecord) error

	// AdvancePointer advances a chat pointer for a chat member
	AdvancePointer(ctx context.Context, chatId ChatId, memberId MemberId, pointerType PointerType, pointer MessageId) error

	// SetMuteState updates the mute state for a chat member
	SetMuteState(ctx context.Context, chatId ChatId, memberId MemberId, isMuted bool) error

	// SetSubscriptionState updates the subscription state for a chat member
	SetSubscriptionState(ctx context.Context, chatId ChatId, memberId MemberId, isSubscribed bool) error
}
