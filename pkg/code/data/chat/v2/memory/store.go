package memory

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"strings"
	"sync"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	chats    map[string]*chat.MetadataRecord
	members  map[string]map[string]*chat.MemberRecord
	messages map[string][]*chat.MessageRecord
}

func New() *InMemoryStore {
	return &InMemoryStore{
		chats:    make(map[string]*chat.MetadataRecord),
		members:  make(map[string]map[string]*chat.MemberRecord),
		messages: make(map[string][]*chat.MessageRecord),
	}
}

// GetChatMetadata retrieves the metadata record for a specific chat
func (s *InMemoryStore) GetChatMetadata(_ context.Context, chatId chat.ChatId) (*chat.MetadataRecord, error) {
	if err := chatId.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if md, exists := s.chats[string(chatId[:])]; exists {
		cloned := md.Clone()
		return &cloned, nil
	}

	return nil, chat.ErrChatNotFound
}

// GetChatMessageV2 retrieves a specific message from a chat
func (s *InMemoryStore) GetChatMessageV2(_ context.Context, chatId chat.ChatId, messageId chat.MessageId) (*chat.MessageRecord, error) {
	if err := chatId.Validate(); err != nil {
		return nil, err
	}
	if err := messageId.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if messages, exists := s.messages[string(chatId[:])]; exists {
		for _, message := range messages {
			if bytes.Equal(message.MessageId[:], messageId[:]) {
				clone := message.Clone()
				return &clone, nil
			}
		}
	}

	return nil, chat.ErrMessageNotFound
}

// GetAllChatsForUserV2 retrieves all chat IDs that a given user belongs to
func (s *InMemoryStore) GetAllChatsForUserV2(_ context.Context, user chat.MemberId, opts ...query.Option) ([]chat.ChatId, error) {
	if err := user.Validate(); err != nil {
		return nil, err
	}

	qo := &query.QueryOptions{
		Supported: query.CanQueryByCursor | query.CanLimitResults | query.CanSortBy,
	}
	err := qo.Apply(opts...)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var chatIds []chat.ChatId
	for chatIdStr, members := range s.members {
		if _, exists := members[user.String()]; exists {
			chatId, _ := chat.GetChatIdFromBytes([]byte(chatIdStr))
			chatIds = append(chatIds, chatId)
		}
	}

	// Sort the chatIds
	sort.Slice(chatIds, func(i, j int) bool {
		if qo.SortBy == query.Descending {
			return bytes.Compare(chatIds[i][:], chatIds[j][:]) > 0
		}
		return bytes.Compare(chatIds[i][:], chatIds[j][:]) < 0
	})

	// Apply cursor if provided
	if qo.Cursor != nil {
		cursorChatId, err := chat.GetChatIdFromBytes(qo.Cursor)
		if err != nil {
			return nil, err
		}
		var filteredChatIds []chat.ChatId
		for _, chatId := range chatIds {
			if qo.SortBy == query.Descending {
				if bytes.Compare(chatId[:], cursorChatId[:]) < 0 {
					filteredChatIds = append(filteredChatIds, chatId)
				}
			} else {
				if bytes.Compare(chatId[:], cursorChatId[:]) > 0 {
					filteredChatIds = append(filteredChatIds, chatId)
				}
			}
		}
		chatIds = filteredChatIds
	}

	// Apply limit if provided
	if qo.Limit > 0 && uint64(len(chatIds)) > qo.Limit {
		chatIds = chatIds[:qo.Limit]
	}

	return chatIds, nil
}

// GetAllChatMessagesV2 retrieves all messages for a specific chat
func (s *InMemoryStore) GetAllChatMessagesV2(_ context.Context, chatId chat.ChatId, opts ...query.Option) ([]*chat.MessageRecord, error) {
	if err := chatId.Validate(); err != nil {
		return nil, err
	}

	qo := &query.QueryOptions{
		Supported: query.CanLimitResults | query.CanSortBy | query.CanQueryByCursor,
	}
	if err := qo.Apply(opts...); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages, exists := s.messages[string(chatId[:])]
	if !exists {
		return nil, nil
	}

	var result []*chat.MessageRecord
	for _, msg := range messages {
		cloned := msg.Clone()
		result = append(result, &cloned)
	}

	// Sort the messages
	sort.Slice(result, func(i, j int) bool {
		if qo.SortBy == query.Descending {
			return bytes.Compare(result[i].MessageId[:], result[j].MessageId[:]) > 0
		}
		return bytes.Compare(result[i].MessageId[:], result[j].MessageId[:]) < 0
	})

	// Apply cursor if provided
	if len(qo.Cursor) > 0 {
		cursorMessageId, err := chat.GetMessageIdFromBytes(qo.Cursor)
		if err != nil {
			return nil, err
		}
		var filteredMessages []*chat.MessageRecord
		for _, msg := range result {
			if qo.SortBy == query.Descending {
				if bytes.Compare(msg.MessageId[:], cursorMessageId[:]) < 0 {
					filteredMessages = append(filteredMessages, msg)
				}
			} else {
				if bytes.Compare(msg.MessageId[:], cursorMessageId[:]) > 0 {
					filteredMessages = append(filteredMessages, msg)
				}
			}
		}
		result = filteredMessages
	}

	// Apply limit if provided
	if qo.Limit > 0 && uint64(len(result)) > qo.Limit {
		result = result[:qo.Limit]
	}

	return result, nil
}

// GetChatMembersV2 retrieves all members of a specific chat
func (s *InMemoryStore) GetChatMembersV2(_ context.Context, chatId chat.ChatId) ([]*chat.MemberRecord, error) {
	if err := chatId.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	members, exists := s.members[string(chatId[:])]
	if !exists {
		return nil, chat.ErrChatNotFound
	}

	var result []*chat.MemberRecord
	for _, member := range members {
		cloned := member.Clone()
		result = append(result, &cloned)
	}

	slices.SortFunc(result, func(a, b *chat.MemberRecord) int {
		return strings.Compare(a.MemberId, b.MemberId)
	})

	return result, nil
}

// IsChatMember checks if a given member is part of a specific chat
func (s *InMemoryStore) IsChatMember(_ context.Context, chatId chat.ChatId, memberId chat.MemberId) (bool, error) {
	if err := chatId.Validate(); err != nil {
		return false, err
	}
	if err := memberId.Validate(); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if members, exists := s.members[string(chatId[:])]; exists {
		_, exists = members[memberId.String()]
		return exists, nil
	}

	return false, nil
}

// PutChatV2 stores or updates the metadata for a specific chat
func (s *InMemoryStore) PutChatV2(_ context.Context, record *chat.MetadataRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.chats[string(record.ChatId[:])]; exists {
		return chat.ErrChatExists
	}

	cloned := record.Clone()
	s.chats[string(record.ChatId[:])] = &cloned

	return nil
}

// PutChatMemberV2 stores or updates a member record for a specific chat
func (s *InMemoryStore) PutChatMemberV2(_ context.Context, record *chat.MemberRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.members[string(record.ChatId[:])]
	if !exists {
		members = make(map[string]*chat.MemberRecord)
		s.members[string(record.ChatId[:])] = members
	}

	if _, exists = members[record.MemberId]; exists {
		return chat.ErrMemberExists
	}

	cloned := record.Clone()
	members[record.MemberId] = &cloned

	return nil
}

// PutChatMessageV2 stores or updates a message record in a specific chat
func (s *InMemoryStore) PutChatMessageV2(_ context.Context, record *chat.MessageRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.messages[string(record.ChatId[:])]
	if messages == nil {
		messages = make([]*chat.MessageRecord, 0)
		s.messages[string(record.ChatId[:])] = messages
	}

	i, found := sort.Find(len(messages), func(i int) int {
		return bytes.Compare(record.MessageId[:], messages[i].MessageId[:])
	})
	if found {
		return chat.ErrMessageExists
	}

	cloned := record.Clone()
	messages = slices.Insert(messages, i, &cloned)
	s.messages[string(record.ChatId[:])] = messages

	return nil
}

// SetChatMuteStateV2 sets the mute state for a specific chat member
func (s *InMemoryStore) SetChatMuteStateV2(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, isMuted bool) error {
	if err := chatId.Validate(); err != nil {
		return err
	}
	if err := memberId.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if members, exists := s.members[string(chatId[:])]; exists {
		if member, exists := members[memberId.String()]; exists {
			member.IsMuted = isMuted
			return nil
		}
	}
	return chat.ErrMemberNotFound
}

// AdvanceChatPointerV2 advances a pointer for a chat member
func (s *InMemoryStore) AdvanceChatPointerV2(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, pointerType chat.PointerType, pointer chat.MessageId) (bool, error) {
	if err := chatId.Validate(); err != nil {
		return false, err
	}
	if err := memberId.Validate(); err != nil {
		return false, err
	}
	if err := pointer.Validate(); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.members[string(chatId[:])]
	if !exists {
		return false, chat.ErrMemberNotFound
	}

	member, exists := members[memberId.String()]
	if !exists {
		return false, chat.ErrMemberNotFound
	}

	switch pointerType {
	case chat.PointerTypeSent:
	case chat.PointerTypeDelivered:
		if member.DeliveryPointer == nil || bytes.Compare(pointer[:], member.DeliveryPointer[:]) > 0 {
			newPtr := pointer.Clone()
			member.DeliveryPointer = &newPtr
			return true, nil
		}
	case chat.PointerTypeRead:
		if member.ReadPointer == nil || bytes.Compare(pointer[:], member.ReadPointer[:]) > 0 {
			newPtr := pointer.Clone()
			member.ReadPointer = &newPtr
			return true, nil
		}
	default:
		return false, chat.ErrInvalidPointerType
	}

	return false, nil
}

// GetChatUnreadCountV2 calculates and returns the unread message count
func (s *InMemoryStore) GetChatUnreadCountV2(ctx context.Context, chatId chat.ChatId, memberId chat.MemberId, readPointer *chat.MessageId) (uint32, error) {
	if err := chatId.Validate(); err != nil {
		return 0, err
	}
	if err := memberId.Validate(); err != nil {
		return 0, err
	}
	if readPointer != nil {
		if err := readPointer.Validate(); err != nil {
			return 0, err
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	unread := uint32(0)
	messages := s.messages[string(chatId[:])]
	for _, message := range messages {
		if readPointer != nil {
			if bytes.Compare(message.MessageId[:], readPointer[:]) <= 0 {
				continue
			}
		}

		if message.Sender.String() == memberId.String() {
			continue
		}

		unread++
	}

	return unread, nil
}
