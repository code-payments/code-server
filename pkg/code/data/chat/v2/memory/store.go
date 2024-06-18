package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
)

// todo: finish implementing me
type store struct {
	mu sync.Mutex

	chatRecords    []*chat.ChatRecord
	memberRecords  []*chat.MemberRecord
	messageRecords []*chat.MessageRecord

	lastChatId    int64
	lastMemberId  int64
	lastMessageId int64
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

// GetAllMembersByChatId implements chat.Store.GetAllMembersByChatId
func (s *store) GetAllMembersByChatId(_ context.Context, chatId chat.ChatId) ([]*chat.MemberRecord, error) {
	items := s.findMembersByChatId(chatId)
	if len(items) == 0 {
		return nil, chat.ErrMemberNotFound
	}
	return cloneMemberRecords(items), nil
}

// GetAllMembersByPlatformIds implements chat.store.GetAllMembersByPlatformIds
func (s *store) GetAllMembersByPlatformIds(_ context.Context, idByPlatform map[chat.Platform]string, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MemberRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findMembersByPlatformIds(idByPlatform)
	items, err := s.getMemberRecordPage(items, cursor, direction, limit)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, chat.ErrMemberNotFound
	}
	return cloneMemberRecords(items), nil
}

// GetUnreadCount implements chat.store.GetUnreadCount
func (s *store) GetUnreadCount(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, readPointer chat.MessageId) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findMessagesByChatId(chatId)
	items = s.filterMessagesAfter(items, readPointer)
	items = s.filterMessagesNotSentBy(items, memberId)
	items = s.filterNotifiedMessages(items)
	return uint32(len(items)), nil
}

// GetAllMessagesByChatId implements chat.Store.GetAllMessagesByChatId
func (s *store) GetAllMessagesByChatId(_ context.Context, chatId chat.ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findMessagesByChatId(chatId)
	items, err := s.getMessageRecordPage(items, cursor, direction, limit)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, chat.ErrMessageNotFound
	}
	return cloneMessageRecords(items), nil
}

// PutChat creates a new chat
func (s *store) PutChat(_ context.Context, record *chat.ChatRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastChatId++

	if item := s.findChat(record); item != nil {
		return chat.ErrChatExists
	}

	record.Id = s.lastChatId
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	cloned := record.Clone()
	s.chatRecords = append(s.chatRecords, &cloned)

	return nil
}

// PutMember creates a new chat member
func (s *store) PutMember(_ context.Context, record *chat.MemberRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastMemberId++

	if item := s.findMember(record); item != nil {
		return chat.ErrMemberExists
	}

	record.Id = s.lastMemberId
	if record.JoinedAt.IsZero() {
		record.JoinedAt = time.Now()
	}

	cloned := record.Clone()
	s.memberRecords = append(s.memberRecords, &cloned)

	return nil
}

// PutMessage implements chat.Store.PutMessage
func (s *store) PutMessage(_ context.Context, record *chat.MessageRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastMessageId++

	if item := s.findMessage(record); item != nil {
		return chat.ErrMessageExsits
	}

	record.Id = s.lastMessageId

	cloned := record.Clone()
	s.messageRecords = append(s.messageRecords, &cloned)

	return nil
}

// AdvancePointer implements chat.Store.AdvancePointer
func (s *store) AdvancePointer(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, pointerType chat.PointerType, pointer chat.MessageId) (bool, error) {
	switch pointerType {
	case chat.PointerTypeDelivered, chat.PointerTypeRead:
	default:
		return false, chat.ErrInvalidPointerType
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return false, chat.ErrMemberNotFound
	}

	var currentPointer *chat.MessageId
	switch pointerType {
	case chat.PointerTypeDelivered:
		currentPointer = item.DeliveryPointer
	case chat.PointerTypeRead:
		currentPointer = item.ReadPointer
	}

	if currentPointer == nil || currentPointer.Before(pointer) {
		switch pointerType {
		case chat.PointerTypeDelivered:
			cloned := pointer.Clone()
			item.DeliveryPointer = &cloned
		case chat.PointerTypeRead:
			cloned := pointer.Clone()
			item.ReadPointer = &cloned
		}

		return true, nil
	}
	return false, nil
}

// UpgradeIdentity implements chat.Store.UpgradeIdentity
func (s *store) UpgradeIdentity(_ context.Context, chatId chat.ChatId, memberId chat.MemberId, platform chat.Platform, platformId string) error {
	switch platform {
	case chat.PlatformTwitter:
	default:
		return errors.Errorf("platform not supported for identity upgrades: %s", platform.String())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMemberById(chatId, memberId)
	if item == nil {
		return chat.ErrMemberNotFound
	}
	if item.Platform != chat.PlatformCode {
		return chat.ErrMemberIdentityAlreadyUpgraded
	}

	item.Platform = platform
	item.PlatformId = platformId

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

func (s *store) findChat(data *chat.ChatRecord) *chat.ChatRecord {
	for _, item := range s.chatRecords {
		if data.Id == item.Id {
			return item
		}

		if bytes.Equal(data.ChatId[:], item.ChatId[:]) {
			return item
		}
	}
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

func (s *store) findMember(data *chat.MemberRecord) *chat.MemberRecord {
	for _, item := range s.memberRecords {
		if data.Id == item.Id {
			return item
		}

		if bytes.Equal(data.ChatId[:], item.ChatId[:]) && bytes.Equal(data.MemberId[:], item.MemberId[:]) {
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

func (s *store) findMembersByChatId(chatId chat.ChatId) []*chat.MemberRecord {
	var res []*chat.MemberRecord
	for _, item := range s.memberRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findMembersByPlatformIds(idByPlatform map[chat.Platform]string) []*chat.MemberRecord {
	var res []*chat.MemberRecord
	for _, item := range s.memberRecords {
		platformId, ok := idByPlatform[item.Platform]
		if !ok {
			continue
		}

		if platformId == item.PlatformId {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) getMemberRecordPage(items []*chat.MemberRecord, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MemberRecord, error) {
	if len(items) == 0 {
		return nil, nil
	}

	var memberIdCursor *uint64
	if len(cursor) > 0 {
		cursorValue := query.FromCursor(cursor)
		memberIdCursor = &cursorValue
	}

	var res []*chat.MemberRecord
	if memberIdCursor == nil {
		res = items
	} else {
		for _, item := range items {
			if item.Id > int64(*memberIdCursor) && direction == query.Ascending {
				res = append(res, item)
			}

			if item.Id < int64(*memberIdCursor) && direction == query.Descending {
				res = append(res, item)
			}
		}
	}

	if direction == query.Ascending {
		sort.Sort(chat.MembersById(res))
	} else {
		sort.Sort(sort.Reverse(chat.MembersById(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit], nil
	}

	return res, nil
}

func (s *store) findMessage(data *chat.MessageRecord) *chat.MessageRecord {
	for _, item := range s.messageRecords {
		if data.Id == item.Id {
			return item
		}

		if bytes.Equal(data.ChatId[:], item.ChatId[:]) && bytes.Equal(data.MessageId[:], item.MessageId[:]) {
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

func (s *store) findMessagesByChatId(chatId chat.ChatId) []*chat.MessageRecord {
	var res []*chat.MessageRecord
	for _, item := range s.messageRecords {
		if bytes.Equal(chatId[:], item.ChatId[:]) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterMessagesAfter(items []*chat.MessageRecord, pointer chat.MessageId) []*chat.MessageRecord {
	var res []*chat.MessageRecord
	for _, item := range items {
		if item.MessageId.After(pointer) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterMessagesNotSentBy(items []*chat.MessageRecord, sender chat.MemberId) []*chat.MessageRecord {
	var res []*chat.MessageRecord
	for _, item := range items {
		if item.Sender == nil || !bytes.Equal(item.Sender[:], sender[:]) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterNotifiedMessages(items []*chat.MessageRecord) []*chat.MessageRecord {
	var res []*chat.MessageRecord
	for _, item := range items {
		if !item.IsSilent {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) getMessageRecordPage(items []*chat.MessageRecord, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.MessageRecord, error) {
	if len(items) == 0 {
		return nil, nil
	}

	var messageIdCursor *chat.MessageId
	if len(cursor) > 0 {
		messageId, err := chat.GetMessageIdFromBytes(cursor)
		if err != nil {
			return nil, err
		}
		messageIdCursor = &messageId
	}

	var res []*chat.MessageRecord
	if messageIdCursor == nil {
		res = items
	} else {
		for _, item := range items {
			if item.MessageId.After(*messageIdCursor) && direction == query.Ascending {
				res = append(res, item)
			}

			if item.MessageId.Before(*messageIdCursor) && direction == query.Descending {
				res = append(res, item)
			}
		}
	}

	if direction == query.Ascending {
		sort.Sort(chat.MessagesByMessageId(res))
	} else {
		sort.Sort(sort.Reverse(chat.MessagesByMessageId(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit], nil
	}

	return res, nil
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

func cloneMemberRecords(items []*chat.MemberRecord) []*chat.MemberRecord {
	res := make([]*chat.MemberRecord, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}

func cloneMessageRecords(items []*chat.MessageRecord) []*chat.MessageRecord {
	res := make([]*chat.MessageRecord, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}
