package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed chat.Store
func New(db *sql.DB) chat.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// PutChat implements chat.Store.PutChat
func (s *store) PutChat(ctx context.Context, record *chat.Chat) error {
	model, err := toChatModel(record)
	if err != nil {
		return err
	}

	err = model.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	fromChatModel(model).CopyTo(record)

	return nil
}

// GetChatById implements chat.Store.GetChatById
func (s *store) GetChatById(ctx context.Context, id chat.ChatId) (*chat.Chat, error) {
	model, err := dbGetChatById(ctx, s.db, id)
	if err != nil {
		return nil, err
	}

	return fromChatModel(model), nil
}

// GetAllChatsForUser implements chat.Store.GetAllChatsForUser
func (s *store) GetAllChatsForUser(ctx context.Context, user string, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.Chat, error) {
	models, err := dbGetAllChatsForUser(ctx, s.db, user, cursor, direction, limit)
	if err != nil {
		return nil, err
	}

	var res []*chat.Chat
	for _, model := range models {
		res = append(res, fromChatModel(model))
	}
	return res, nil
}

// PutMessage implements chat.Store.PutMessage
func (s *store) PutMessage(ctx context.Context, record *chat.Message) error {
	model, err := toMessageModel(record)
	if err != nil {
		return err
	}

	err = model.dbPut(ctx, s.db)
	if err != nil {
		return err
	}

	fromMessageModel(model).CopyTo(record)

	return nil
}

// DeleteMessage implements chat.Store.DeleteMessage
func (s *store) DeleteMessage(ctx context.Context, chatId chat.ChatId, messageId string) error {
	return dbDeleteMessage(ctx, s.db, chatId, messageId)
}

// GetMessageById implements chat.Store.GetMessageById
func (s *store) GetMessageById(ctx context.Context, chatId chat.ChatId, messageId string) (*chat.Message, error) {
	model, err := dbGetMessageById(ctx, s.db, chatId, messageId)
	if err != nil {
		return nil, err
	}

	return fromMessageModel(model), nil
}

// GetAllMessagesByChat implements chat.Store.GetAllMessagesByChat
func (s *store) GetAllMessagesByChat(ctx context.Context, chatId chat.ChatId, cursor query.Cursor, direction query.Ordering, limit uint64) ([]*chat.Message, error) {
	models, err := dbGetAllMessagesByChat(ctx, s.db, chatId, cursor, direction, limit)
	if err != nil {
		return nil, err
	}

	var res []*chat.Message
	for _, model := range models {
		res = append(res, fromMessageModel(model))
	}
	return res, nil
}

// AdvancePointer implements chat.Store.AdvancePointer
func (s *store) AdvancePointer(ctx context.Context, chatId chat.ChatId, pointer string) error {
	return dbAdvancePointer(ctx, s.db, chatId, pointer)
}

// GetUnreadCount implements chat.Store.GetUnreadCount
func (s *store) GetUnreadCount(ctx context.Context, chatId chat.ChatId) (uint32, error) {
	return dbGetUnreadCount(ctx, s.db, chatId)
}

// SetMuteState implements chat.Store.SetMuteState
func (s *store) SetMuteState(ctx context.Context, chatId chat.ChatId, isMuted bool) error {
	return dbSetMuteState(ctx, s.db, chatId, isMuted)
}

// SetSubscriptionState implements chat.Store.SetSubscriptionState
func (s *store) SetSubscriptionState(ctx context.Context, chatId chat.ChatId, isSubscribed bool) error {
	return dbSetSubscriptionState(ctx, s.db, chatId, isSubscribed)
}
