package memory

import (
	"context"
	"errors"
	"sync"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
)

// todo: implement me
type store struct {
	mu   sync.Mutex
	last uint64
}

// New returns a new in memory chat.Store
func New() chat.Store {
	return &store{}
}

// GetChatById implements chat.Store.GetChatById
func (s *store) GetChatById(ctx context.Context, chatId chat.ChatId) (*chat.ChatRecord, error) {
	return nil, errors.New("not implemented")
}

// GetMemberById implements chat.Store.GetMemberById
func (s *store) GetMemberById(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId) (*chat.MemberRecord, error) {
	return nil, errors.New("not implemented")
}

// GetMessageById implements chat.Store.GetMessageById
func (s *store) GetMessageById(ctx context.Context, chatId chat.ChatId, messageId chat.MessageId) (*chat.MessageRecord, error) {
	return nil, errors.New("not implemented")
}

// GetAllMessagesByChat implements chat.Store.GetAllMessagesByChat
func (s *store) GetAllMessagesByChat(ctx context.Context, chatId chat.ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MessageRecord, error) {
	return nil, errors.New("not implemented")
}

// PutChat creates a new chat
func (s *store) PutChat(ctx context.Context, record *chat.ChatRecord) error {
	return errors.New("not implemented")
}

// PutMember creates a new chat member
func (s *store) PutMember(ctx context.Context, record *chat.MemberRecord) error {
	return errors.New("not implemented")
}

// PutMessage implements chat.Store.PutMessage
func (s *store) PutMessage(ctx context.Context, record *chat.MessageRecord) error {
	return errors.New("not implemented")
}

// AdvancePointer implements chat.Store.AdvancePointer
func (s *store) AdvancePointer(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, pointerType chat.PointerType, pointer chat.MessageId) error {
	return errors.New("not implemented")
}

// SetMuteState implements chat.Store.SetMuteState
func (s *store) SetMuteState(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, isMuted bool) error {
	return errors.New("not implemented")
}

// SetSubscriptionState implements chat.Store.SetSubscriptionState
func (s *store) SetSubscriptionState(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, isSubscribed bool) error {
	return errors.New("not implemented")
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.last = 0
}
