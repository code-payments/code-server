package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	chatTableName    = "codewallet__core_chat"
	messageTableName = "codewallet__core_chatmessage"
)

type chatModel struct {
	Id sql.NullInt64 `db:"id"`

	ChatId     []byte `db:"chat_id"`
	ChatType   uint8  `db:"chat_type"`
	IsVerified bool   `db:"is_verified"`

	Member1 string `db:"member1"`
	Member2 string `db:"member2"` // Keeping this open in case we want to have bidirectional chats across users

	ReadPointer    sql.NullString `db:"read_pointer"`
	IsMuted        bool           `db:"is_muted"`
	IsUnsubscribed bool           `db:"is_unsubscribed"`

	CreatedAt time.Time `db:"created_at"`
}

type messageModel struct {
	Id sql.NullInt64 `db:"id"`

	ChatId []byte `db:"chat_id"`

	MessageId string `db:"message_id"`
	Data      []byte `db:"data"`

	IsSilent      bool  `db:"is_silent"`
	ContentLength uint8 `db:"content_length"`

	Timestamp time.Time `db:"timestamp"`
}

func toChatModel(obj *chat.Chat) (*chatModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &chatModel{
		ChatId:     obj.ChatId[:],
		ChatType:   uint8(obj.ChatType),
		IsVerified: obj.IsVerified,

		Member1: obj.CodeUser,
		Member2: obj.ChatTitle,

		ReadPointer: sql.NullString{
			Valid:  obj.ReadPointer != nil,
			String: *pointer.StringOrDefault(obj.ReadPointer, ""),
		},
		IsMuted:        obj.IsMuted,
		IsUnsubscribed: obj.IsUnsubscribed,

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromChatModel(obj *chatModel) *chat.Chat {
	var chatId chat.ChatId
	copy(chatId[:], obj.ChatId)

	return &chat.Chat{
		Id: uint64(obj.Id.Int64),

		ChatId:     chatId,
		ChatType:   chat.ChatType(obj.ChatType),
		IsVerified: obj.IsVerified,

		CodeUser:  obj.Member1,
		ChatTitle: obj.Member2,

		ReadPointer:    pointer.StringIfValid(obj.ReadPointer.Valid, obj.ReadPointer.String),
		IsMuted:        obj.IsMuted,
		IsUnsubscribed: obj.IsUnsubscribed,

		CreatedAt: obj.CreatedAt,
	}
}

func toMessageModel(obj *chat.Message) (*messageModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &messageModel{
		ChatId: obj.ChatId[:],

		MessageId: obj.MessageId,
		Data:      obj.Data,

		IsSilent:      obj.IsSilent,
		ContentLength: obj.ContentLength,

		Timestamp: obj.Timestamp,
	}, nil
}

func fromMessageModel(obj *messageModel) *chat.Message {
	var chatId chat.ChatId
	copy(chatId[:], obj.ChatId)

	return &chat.Message{
		Id: uint64(obj.Id.Int64),

		ChatId: chatId,

		MessageId: obj.MessageId,
		Data:      obj.Data,

		IsSilent:      obj.IsSilent,
		ContentLength: obj.ContentLength,

		Timestamp: obj.Timestamp,
	}
}

func (m *chatModel) dbPut(ctx context.Context, db *sqlx.DB) error {
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + chatTableName + `
			(chat_id, chat_type, is_verified, member1, member2, read_pointer, is_muted, is_unsubscribed, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, chat_id, chat_type, is_verified, member1, member2, created_at
		`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		return tx.QueryRowxContext(
			ctx,
			query,
			m.ChatId,
			m.ChatType,
			m.IsVerified,
			m.Member1,
			m.Member2,
			m.ReadPointer,
			m.IsMuted,
			m.IsUnsubscribed,
			m.CreatedAt,
		).StructScan(m)
	})
	return pgutil.CheckUniqueViolation(err, chat.ErrChatAlreadyExists)
}

func (m *messageModel) dbPut(ctx context.Context, db *sqlx.DB) error {
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + messageTableName + `
			(chat_id, message_id, data, is_silent, content_length, timestamp)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, chat_id, message_id, data, timestamp
		`

		return tx.QueryRowxContext(
			ctx,
			query,
			m.ChatId,
			m.MessageId,
			m.Data,
			m.IsSilent,
			m.ContentLength,
			m.Timestamp,
		).StructScan(m)
	})
	return pgutil.CheckUniqueViolation(err, chat.ErrMessageAlreadyExists)
}

func dbGetChatById(ctx context.Context, db *sqlx.DB, id chat.ChatId) (*chatModel, error) {
	res := &chatModel{}

	query := `SELECT id, chat_id, chat_type, is_verified, member1, member2, read_pointer, is_muted, is_unsubscribed, created_at FROM ` + chatTableName + `
		WHERE chat_id = $1
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		id[:],
	).StructScan(res)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, chat.ErrChatNotFound)
	}
	return res, nil
}

func dbDeleteMessage(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, messageId string) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `DELETE FROM ` + messageTableName + `
			WHERE chat_id = $1 AND message_id = $2
		`

		_, err := db.ExecContext(
			ctx,
			query,
			chatId[:],
			messageId,
		)
		return err
	})
}

func dbGetMessageById(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, messageId string) (*messageModel, error) {
	res := &messageModel{}

	query := `SELECT id, chat_id, message_id, data, is_silent, content_length, timestamp FROM ` + messageTableName + `
		WHERE chat_id = $1 AND message_id = $2
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		chatId[:],
		messageId,
	).StructScan(res)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, chat.ErrMessageNotFound)
	}
	return res, nil
}

func dbGetAllChatsForUser(ctx context.Context, db *sqlx.DB, user string, cursor q.Cursor, direction q.Ordering, limit uint64) ([]*chatModel, error) {
	res := []*chatModel{}

	query := `SELECT id, chat_id, chat_type, is_verified, member1, member2, read_pointer, is_muted, is_unsubscribed, created_at FROM ` + chatTableName + `
		WHERE member1 = $1`

	opts := []interface{}{user}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(
		ctx,
		&res,
		query,
		opts...,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, chat.ErrChatNotFound)
	} else if len(res) == 0 {
		return nil, chat.ErrChatNotFound
	}
	return res, nil
}

func dbGetAllMessagesByChat(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, cursor q.Cursor, direction q.Ordering, limit uint64) ([]*messageModel, error) {
	res := []*messageModel{}

	query := `SELECT id, chat_id, message_id, data, is_silent, content_length, timestamp FROM ` + messageTableName + `
		WHERE chat_id = $1`

	opts := []interface{}{chatId[:]}

	if len(cursor) > 0 {
		// todo: optimize to a single query
		messageModel, err := dbGetMessageById(ctx, db, chatId, cursor.ToBase58())
		if err == chat.ErrMessageNotFound {
			return nil, chat.ErrInvalidMessageCursor
		}

		if direction == q.Ascending {
			query += fmt.Sprintf(" AND (timestamp > $%d OR (timestamp = $%d AND id > $%d))", len(opts)+1, len(opts)+1, len(opts)+2)
		} else {
			query += fmt.Sprintf(" AND (timestamp < $%d OR (timestamp = $%d AND id < $%d))", len(opts)+1, len(opts)+1, len(opts)+2)
		}

		opts = append(opts, messageModel.Timestamp, messageModel.Id)
	}

	// Optimize for timestamp, but in the event messages fall in the same time,
	// fall back to DB insertion order.
	if direction == q.Ascending {
		query += " ORDER BY timestamp ASC, id ASC"
	} else {
		query += " ORDER BY timestamp DESC, id DESC"
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", len(opts)+1)
		opts = append(opts, limit)
	}

	err := db.SelectContext(
		ctx,
		&res,
		query,
		opts...,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, chat.ErrMessageNotFound)
	} else if len(res) == 0 {
		return nil, chat.ErrMessageNotFound
	}
	return res, nil
}

func dbAdvancePointer(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, pointer string) error {
	query := `UPDATE ` + chatTableName + `
		SET read_pointer = $2
		WHERE chat_id = $1
	`

	res, err := db.ExecContext(
		ctx,
		query,
		chatId[:],
		pointer,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	} else if rowsAffected == 0 {
		return chat.ErrChatNotFound
	}

	return nil
}

func dbGetUnreadCount(ctx context.Context, db *sqlx.DB, chatId chat.ChatId) (uint32, error) {
	res := &struct {
		UnreadCount sql.NullInt64 `db:"unread_count"`
	}{}

	query := `SELECT COUNT(*) AS unread_count FROM ` + messageTableName + ` WHERE
		chat_id = $1 AND
		timestamp > COALESCE((SELECT timestamp FROM ` + messageTableName + ` WHERE chat_id = $1 AND message_id = (SELECT read_pointer FROM ` + chatTableName + ` WHERE chat_id = $1)), $2)
		AND NOT is_silent
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		chatId[:],
		time.Time{},
	).StructScan(res)
	if err != nil {
		return 0, err
	}
	return uint32(res.UnreadCount.Int64), nil
}

func dbSetMuteState(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, isMuted bool) error {
	query := `UPDATE ` + chatTableName + `
		SET is_muted = $2
		WHERE chat_id = $1
	`

	res, err := db.ExecContext(
		ctx,
		query,
		chatId[:],
		isMuted,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	} else if rowsAffected == 0 {
		return chat.ErrChatNotFound
	}

	return nil
}

func dbSetSubscriptionState(ctx context.Context, db *sqlx.DB, chatId chat.ChatId, isSubscribed bool) error {
	query := `UPDATE ` + chatTableName + `
		SET is_unsubscribed = $2
		WHERE chat_id = $1
	`

	res, err := db.ExecContext(
		ctx,
		query,
		chatId[:],
		!isSubscribed,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	} else if rowsAffected == 0 {
		return chat.ErrChatNotFound
	}

	return nil
}
