package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/database/query"
)

type ChatsById []*chat.Chat

func (a ChatsById) Len() int           { return len(a) }
func (a ChatsById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ChatsById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type MessagesByTimestampAndId []*chat.Message

func (a MessagesByTimestampAndId) Len() int      { return len(a) }
func (a MessagesByTimestampAndId) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MessagesByTimestampAndId) Less(i, j int) bool {
	if a[i].Timestamp.Before(a[j].Timestamp) {
		return true
	}

	if a[i].Timestamp.Equal(a[j].Timestamp) && a[i].Id < a[j].Id {
		return true
	}

	return false
}

type store struct {
	mu             sync.Mutex
	chatRecords    []*chat.Chat
	messageRecords []*chat.Message
	last           uint64
}

// New returns a new in memory chat.Store
func New() chat.Store {
	return &store{}
}

// PutChat implements chat.Store.PutChat
func (s *store) PutChat(ctx context.Context, data *chat.Chat) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.findChat(data); item != nil {
		return chat.ErrChatAlreadyExists
	}

	data.Id = s.last
	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now()
	}

	cloned := data.Clone()
	s.chatRecords = append(s.chatRecords, &cloned)

	return nil
}

// GetChatById implements chat.Store.GetChatById
func (s *store) GetChatById(ctx context.Context, id chat.ChatId) (*chat.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findChatById(id)
	if item == nil {
		return nil, chat.ErrChatNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetAllChatsForUser implements chat.Store.GetAllChatsForUser
func (s *store) GetAllChatsForUser(ctx context.Context, user string, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findChatsByUser(user)
	items = s.filterPagedChats(items, cursor, direction, limit)
	if len(items) == 0 {
		return nil, chat.ErrChatNotFound
	}

	return cloneChats(items), nil
}

// PutMessage implements chat.Store.PutMessage
func (s *store) PutMessage(ctx context.Context, data *chat.Message) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.findMessage(data); item != nil {
		return chat.ErrMessageAlreadyExists
	}

	data.Id = s.last

	cloned := data.Clone()
	s.messageRecords = append(s.messageRecords, &cloned)

	return nil
}

// DeleteMessage implements chat.Store.DeleteMessage
func (s *store) DeleteMessage(ctx context.Context, chatId chat.ChatId, messageId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.messageRecords {
		if bytes.Equal(item.ChatId[:], chatId[:]) && item.MessageId == messageId {
			s.messageRecords = append(s.messageRecords[:i], s.messageRecords[i+1:]...)
			return nil
		}
	}

	return nil
}

// GetMessageById implements chat.Store.GetMessageById
func (s *store) GetMessageById(ctx context.Context, chatId chat.ChatId, messageId string) (*chat.Message, error) {
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
func (s *store) GetAllMessagesByChat(ctx context.Context, chatId chat.ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findMessagesByChatId(chatId)
	items, err := s.filterPagedMessagesByChat(items, cursor, direction, limit)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, chat.ErrMessageNotFound
	}

	return cloneMessages(items), nil
}

// AdvancePointer implements chat.Store.AdvancePointer
func (s *store) AdvancePointer(_ context.Context, chatId chat.ChatId, pointer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findChatById(chatId)
	if item == nil {
		return chat.ErrChatNotFound
	}

	item.ReadPointer = &pointer

	return nil
}

// GetUnreadCount implements chat.Store.GetUnreadCount
func (s *store) GetUnreadCount(ctx context.Context, chatId chat.ChatId) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chatItem := s.findChatById(chatId)
	if chatItem == nil {
		return 0, nil
	}

	var after time.Time
	if chatItem.ReadPointer != nil {
		messageItem := s.findMessageById(chatId, *chatItem.ReadPointer)
		if messageItem != nil {
			after = messageItem.Timestamp
		}
	}

	messageItems := s.findMessagesByChatId(chatId)
	messageItems = s.filterMessagesAfter(messageItems, after)
	messageItems = s.filterNotifiedMessages(messageItems)
	return uint32(len(messageItems)), nil
}

// SetMuteState implements chat.Store.SetMuteState
func (s *store) SetMuteState(ctx context.Context, chatId chat.ChatId, isMuted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chatItem := s.findChatById(chatId)
	if chatItem == nil {
		return chat.ErrChatNotFound
	}

	chatItem.IsMuted = isMuted

	return nil
}

// SetSubscriptionState implements chat.Store.SetSubscriptionState
func (s *store) SetSubscriptionState(ctx context.Context, chatId chat.ChatId, isSubscribed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chatItem := s.findChatById(chatId)
	if chatItem == nil {
		return chat.ErrChatNotFound
	}

	chatItem.IsUnsubscribed = !isSubscribed

	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chatRecords = nil
	s.messageRecords = nil
	s.last = 0
}

func (s *store) findChat(data *chat.Chat) *chat.Chat {
	for _, item := range s.chatRecords {
		if item.Id == data.Id {
			return item
		}

		if bytes.Equal(data.ChatId[:], item.ChatId[:]) {
			return item
		}
	}
	return nil
}

func (s *store) findChatById(id chat.ChatId) *chat.Chat {
	for _, item := range s.chatRecords {
		if bytes.Equal(id[:], item.ChatId[:]) {
			return item
		}
	}
	return nil
}

func (s *store) findChatsByUser(user string) []*chat.Chat {
	var res []*chat.Chat
	for _, item := range s.chatRecords {
		if item.CodeUser == user {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findMessage(data *chat.Message) *chat.Message {
	for _, item := range s.messageRecords {
		if item.Id == data.Id {
			return item
		}

		if bytes.Equal(item.ChatId[:], data.ChatId[:]) && item.MessageId == data.MessageId {
			return item
		}
	}
	return nil
}

func (s *store) findMessageById(chatId chat.ChatId, messageId string) *chat.Message {
	for _, item := range s.messageRecords {
		if bytes.Equal(item.ChatId[:], chatId[:]) && item.MessageId == messageId {
			return item
		}
	}
	return nil
}

func (s *store) findMessagesByChatId(chatId chat.ChatId) []*chat.Message {
	var res []*chat.Message
	for _, item := range s.messageRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterMessagesAfter(items []*chat.Message, ts time.Time) []*chat.Message {
	var res []*chat.Message
	for _, item := range items {
		if item.Timestamp.After(ts) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterNotifiedMessages(items []*chat.Message) []*chat.Message {
	var res []*chat.Message
	for _, item := range items {
		if !item.IsSilent {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterPagedChats(items []*chat.Chat, cursor query.Cursor, direction query.Ordering, limit uint64) []*chat.Chat {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*chat.Chat
	for _, item := range items {
		if item.Id > start && direction == query.Ascending {
			res = append(res, item)
		}
		if item.Id < start && direction == query.Descending {
			res = append(res, item)
		}
	}

	if direction == query.Ascending {
		sort.Sort(ChatsById(res))
	} else {
		sort.Sort(sort.Reverse(ChatsById(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit]
	}

	return res
}

func (s *store) filterPagedMessagesByChat(items []*chat.Message, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.Message, error) {
	if len(items) == 0 {
		return nil, nil
	}

	var recordCursor *chat.Message
	if len(cursor) > 0 {
		recordCursor = s.findMessageById(items[0].ChatId, cursor.ToBase58())
		if recordCursor == nil {
			return nil, chat.ErrInvalidMessageCursor
		}
	}

	var res []*chat.Message
	if recordCursor == nil {
		res = items
	} else {
		for _, item := range items {
			if item.Timestamp.Equal(recordCursor.Timestamp) {
				if item.Id > recordCursor.Id && direction == query.Ascending {
					res = append(res, item)
				}

				if item.Id < recordCursor.Id && direction == query.Descending {
					res = append(res, item)
				}
			}

			if item.Timestamp.After(recordCursor.Timestamp) && direction == query.Ascending {
				res = append(res, item)
			}

			if item.Timestamp.Before(recordCursor.Timestamp) && direction == query.Descending {
				res = append(res, item)
			}
		}
	}

	if direction == query.Ascending {
		sort.Sort(MessagesByTimestampAndId(res))
	} else {
		sort.Sort(sort.Reverse(MessagesByTimestampAndId(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit], nil
	}

	return res, nil
}

func (s *store) sumContentLengths(items []*chat.Message) uint32 {
	var res uint32
	for _, item := range items {
		res += uint32(item.ContentLength)
	}
	return res
}

func cloneChats(items []*chat.Chat) []*chat.Chat {
	res := make([]*chat.Chat, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}

func cloneMessages(items []*chat.Message) []*chat.Message {
	res := make([]*chat.Message, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}
