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
	ErrMessageExists      = errors.New("chat message already exists")
	ErrMessageNotFound    = errors.New("chat message not found")
	ErrInvalidPointerType = errors.New("invalid pointer type")
)

type Store interface {
	// GetChatMetadata retrieves the metadata record for a specific chat, identified by chatId.
	//
	// It returns ErrChatNotFound if the chat doesn't exist.
	GetChatMetadata(ctx context.Context, chatId ChatId) (*MetadataRecord, error)

	// GetChatMessageV2 retrieves a specific message from a chat, identified by chatId and messageId.
	GetChatMessageV2(ctx context.Context, chatId ChatId, messageId MessageId) (*MessageRecord, error)

	// GetAllChatsForUserV2 retrieves all chat IDs that a given user (where user is the messaging address).
	GetAllChatsForUserV2(ctx context.Context, user MemberId, opts ...query.Option) ([]ChatId, error)

	// GetAllChatMessagesV2 retrieves all messages for a specific chat, identified by chatId.
	GetAllChatMessagesV2(ctx context.Context, chatId ChatId, opts ...query.Option) ([]*MessageRecord, error)

	// GetChatMembersV2 retrieves all members of a specific chat, identified by chatId.
	GetChatMembersV2(ctx context.Context, chatId ChatId) ([]*MemberRecord, error)

	// IsChatMember checks if a given member, identified by memberId, is part of a specific chat, identified by chatId.
	IsChatMember(ctx context.Context, chatId ChatId, memberId MemberId) (bool, error)

	// PutChatV2 stores or updates the metadata for a specific chat.
	//
	// ErrChatExists is returned if the chat with the same ID already exists.
	PutChatV2(ctx context.Context, record *MetadataRecord) error

	// PutChatMemberV2 stores or updates a member record for a specific chat.
	//
	// ErrMemberExists is returned if the member already exists.
	// Updating should be done with specific DB calls.
	PutChatMemberV2(ctx context.Context, record *MemberRecord) error

	// PutChatMessageV2 stores or updates a message record in a specific chat.
	//
	// ErrMessageExists is returned if the message already exists.
	PutChatMessageV2(ctx context.Context, record *MessageRecord) error

	// SetChatMuteStateV2 sets the mute state for a specific chat member, identified by chatId and memberId.
	//
	// ErrMemberNotFound if the member does not exist.
	SetChatMuteStateV2(ctx context.Context, chatId ChatId, memberId MemberId, isMuted bool) error

	// AdvanceChatPointerV2 advances a pointer for a chat member, identified by chatId and memberId.
	//
	// It returns whether the pointer was advanced. If no member exists, ErrMemberNotFound is returned.
	AdvanceChatPointerV2(ctx context.Context, chatId ChatId, memberId MemberId, pointerType PointerType, pointer MessageId) (bool, error)

	// GetChatUnreadCountV2 calculates and returns the unread message count for a specific chat member,
	//
	// Existence checks are not performed.
	GetChatUnreadCountV2(ctx context.Context, chatId ChatId, memberId MemberId, readPointer *MessageId) (uint32, error)
}
