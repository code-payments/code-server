package chat_v1

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/database/query"
)

var (
	ErrChatAlreadyExists    = errors.New("chat record already exists")
	ErrChatNotFound         = errors.New("chat record not found")
	ErrMessageAlreadyExists = errors.New("message record already exists")
	ErrMessageNotFound      = errors.New("message record not found")
	ErrInvalidMessageCursor = errors.New("message cursor is invalid")
)

type Store interface {
	// PutChat creates a new chat metadata
	PutChat(ctx context.Context, record *Chat) error

	// GetChatById gets a chat by its chat ID
	GetChatById(ctx context.Context, chatId ChatId) (*Chat, error)

	// GetAllChatsForUser gets all chats for a given user
	//
	// Note: Cursor is the auto-incrementing ID
	GetAllChatsForUser(ctx context.Context, user string, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*Chat, error)

	// PutMessage creates a new new chat message
	PutMessage(ctx context.Context, record *Message) error

	// Delete message deletes a message within a chat. The call is idempotent
	// and will not fail if the message doesn't exist.
	DeleteMessage(ctx context.Context, chatId ChatId, messageId string) error

	// GetMessageById gets a chat message by its message ID within a chat
	GetMessageById(ctx context.Context, chatId ChatId, messageId string) (*Message, error)

	// GetAllMessagesByChat gets all messages for a given chat
	//
	// Note: Cursor is a message ID
	GetAllMessagesByChat(ctx context.Context, chatId ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*Message, error)

	// AdvancePointer advances a chat pointer
	AdvancePointer(ctx context.Context, chatId ChatId, pointer string) error

	// GetUnreadCount gets the unread message count for a chat ID
	GetUnreadCount(ctx context.Context, chatId ChatId) (uint32, error)

	// SetMuteState updates the mute state for a chat
	SetMuteState(ctx context.Context, chatId ChatId, isMuted bool) error

	// SetSubscriptionState updates the subscription state for a chat
	SetSubscriptionState(ctx context.Context, chatId ChatId, isSubscribed bool) error
}
