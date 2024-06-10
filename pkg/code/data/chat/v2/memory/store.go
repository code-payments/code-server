package memory

import (
	"bytes"
	"context"
	"errors"
	"sync"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
)

// todo: finish implementing me
type store struct {
	mu sync.Mutex

	chatRecords    []*chat.ChatRecord
	memberRecords  []*chat.MemberRecord
	messageRecords []*chat.MessageRecord

	lastChatId    uint64
	lastMemberId  uint64
	lastMessageId uint64
}

// New returns a new in memory chat.Store
func New() chat.Store {
	return &store{}
}

// GetChatById implements chat.Store.GetChatById
func (s *store) GetChatById(_ context.Context, chatId chat.ChatId) (*chat.ChatRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findChatById(chatId)
	if item == nil {
		return nil, chat.ErrChatNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetMemberById implements chat.Store.GetMemberById
func (s *store) GetMemberById(_ context.Context, chatId chat.ChatId, memberId chat.MemberId) (*chat.MemberRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return nil, chat.ErrMemberNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetMessageById implements chat.Store.GetMessageById
func (s *store) GetMessageById(_ context.Context, chatId chat.ChatId, messageId chat.MessageId) (*chat.MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMessageById(chatId, messageId)
	if item == nil {
		return nil, chat.ErrMessageNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetAllMessagesByChat implements chat.Store.GetAllMessagesByChat
func (s *store) GetAllMessagesByChat(_ context.Context, chatId chat.ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return nil, errors.New("not implemented")
}

// PutChat creates a new chat
func (s *store) PutChat(_ context.Context, record *chat.ChatRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return errors.New("not implemented")
}

// PutMember creates a new chat member
func (s *store) PutMember(_ context.Context, record *chat.MemberRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return errors.New("not implemented")
}

// PutMessage implements chat.Store.PutMessage
func (s *store) PutMessage(_ context.Context, record *chat.MessageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return errors.New("not implemented")
}

// AdvancePointer implements chat.Store.AdvancePointer
func (s *store) AdvancePointer(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, pointerType chat.PointerType, pointer chat.MessageId) error {
	switch pointerType {
	case chat.PointerTypeDelivered, chat.PointerTypeRead:
	default:
		return chat.ErrInvalidPointerType
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return chat.ErrMemberNotFound
	}

	var currentPointer *chat.MessageId
	switch pointerType {
	case chat.PointerTypeDelivered:
		currentPointer = item.DeliveryPointer
	case chat.PointerTypeRead:
		currentPointer = item.ReadPointer
	}

	if currentPointer != nil && currentPointer.After(pointer) {
		return nil
	}

	switch pointerType {
	case chat.PointerTypeDelivered:
		item.DeliveryPointer = &pointer // todo: pointer copy safety
	case chat.PointerTypeRead:
		item.ReadPointer = &pointer // todo: pointer copy safety
	}

	return nil
}

// SetMuteState implements chat.Store.SetMuteState
func (s *store) SetMuteState(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, isMuted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return chat.ErrMemberNotFound
	}

	item.IsMuted = isMuted

	return nil
}

// SetSubscriptionState implements chat.Store.SetSubscriptionState
func (s *store) SetSubscriptionState(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, isSubscribed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return chat.ErrMemberNotFound
	}

	item.IsUnsubscribed = !isSubscribed

	return nil
}

func (s *store) findChatById(chatId chat.ChatId) *chat.ChatRecord {
	for _, item := range s.chatRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) {
			return item
		}
	}
	return nil
}

func (s *store) findMemberById(chatId chat.ChatId, memberId chat.MemberId) *chat.MemberRecord {
	for _, item := range s.memberRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) && bytes.Equal(memberId[:], item.MemberId[:]) {
			return item
		}
	}
	return nil
}

func (s *store) findMessageById(chatId chat.ChatId, messageId chat.MessageId) *chat.MessageRecord {
	for _, item := range s.messageRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) && bytes.Equal(messageId[:], item.MessageId[:]) {
			return item
		}
	}
	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chatRecords = nil
	s.memberRecords = nil
	s.messageRecords = nil

	s.lastChatId = 0
	s.lastMemberId = 0
	s.lastMessageId = 0
}
