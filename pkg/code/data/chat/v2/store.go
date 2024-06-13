package chat_v2

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrChatExists         = errors.New("chat already exists")
	ErrChatNotFound       = errors.New("chat not found")
	ErrMemberExists       = errors.New("chat member already exists")
	ErrMemberNotFound     = errors.New("chat member not found")
	ErrMessageExsits      = errors.New("chat message already exists")
	ErrMessageNotFound    = errors.New("chat message not found")
	ErrInvalidPointerType = errors.New("invalid pointer type")
)

// todo: Define interface methods
type Store interface {
	// GetChatById gets a chat by its chat ID
	GetChatById(ctx context.Context, chatId ChatId) (*ChatRecord, error)

	// GetMemberById gets a chat member by the chat and member IDs
	GetMemberById(ctx context.Context, chatId ChatId, memberId MemberId) (*MemberRecord, error)

	// GetMessageById gets a chat message by the chat and message IDs
	GetMessageById(ctx context.Context, chatId ChatId, messageId MessageId) (*MessageRecord, error)

	// GetAllMembersByChatId gets all members for a given chat
	//
	// todo: Add paging when we introduce group chats
	GetAllMembersByChatId(ctx context.Context, chatId ChatId) ([]*MemberRecord, error)

	// GetAllMembersByPlatformId gets all members for a given platform user across
	// all chats
	GetAllMembersByPlatformId(ctx context.Context, platform Platform, platformId string, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*MemberRecord, error)

	// GetAllMessagesByChatId gets all messages for a given chat
	//
	// Note: Cursor is a message ID
	GetAllMessagesByChatId(ctx context.Context, chatId ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*MessageRecord, error)

	// GetUnreadCount gets the unread message count for a chat ID at a read pointer
	GetUnreadCount(ctx context.Context, chatId ChatId, readPointer MessageId) (uint32, error)

	// PutChat creates a new chat
	PutChat(ctx context.Context, record *ChatRecord) error

	// PutMember creates a new chat member
	PutMember(ctx context.Context, record *MemberRecord) error

	// PutMessage creates a new chat message
	PutMessage(ctx context.Context, record *MessageRecord) error

	// AdvancePointer advances a chat pointer for a chat member
	AdvancePointer(ctx context.Context, chatId ChatId, memberId MemberId, pointerType PointerType, pointer MessageId) error

	// SetMuteState updates the mute state for a chat member
	SetMuteState(ctx context.Context, chatId ChatId, memberId MemberId, isMuted bool) error

	// SetSubscriptionState updates the subscription state for a chat member
	SetSubscriptionState(ctx context.Context, chatId ChatId, memberId MemberId, isSubscribed bool) error
}
