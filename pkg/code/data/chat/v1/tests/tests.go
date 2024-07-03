package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s chat.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s chat.Store){
		testChatRoundTrip,
		testMessageRoundTrip,
		testAdvancePointer,
		testGetUnreadCount,
		testMuteState,
		tesSubscriptionState,
		testGetAllChatsByUserPaging,
		testGetAllMessagesByChatPaging,
	} {
		tf(t, s)
		teardown()
	}
}

func testChatRoundTrip(t *testing.T, s chat.Store) {
	t.Run("testChatRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		chatId := chat.GetChatId("user", "domain", true)

		_, err := s.GetChatById(ctx, chatId)
		assert.Equal(t, chat.ErrChatNotFound, err)

		_, err = s.GetAllChatsForUser(ctx, "user", nil, query.Ascending, 10)
		assert.Equal(t, chat.ErrChatNotFound, err)

		expected := &chat.Chat{
			ChatId:     chatId,
			ChatType:   chat.ChatTypeExternalApp,
			ChatTitle:  "domain",
			IsVerified: true,

			CodeUser: "user",

			ReadPointer:    pointer.String("msg123"),
			IsMuted:        true,
			IsUnsubscribed: true,

			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()

		require.NoError(t, s.PutChat(ctx, expected))
		assert.True(t, expected.Id > 0)

		actual, err := s.GetChatById(ctx, chatId)
		require.NoError(t, err)
		assertEquivalentChatRecords(t, &cloned, actual)

		chats, err := s.GetAllChatsForUser(ctx, "user", nil, query.Ascending, 10)
		require.NoError(t, err)
		require.Len(t, chats, 1)
		assertEquivalentChatRecords(t, &cloned, chats[0])

		assert.Equal(t, chat.ErrChatAlreadyExists, s.PutChat(ctx, expected))
	})
}

func testMessageRoundTrip(t *testing.T, s chat.Store) {
	t.Run("testMessageRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		messageId := "message_id"

		for i := 0; i < 3; i++ {
			chatId := chat.GetChatId("sender", fmt.Sprintf("receiver%d", i), true)

			_, err := s.GetMessageById(ctx, chatId, messageId)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			_, err = s.GetAllMessagesByChat(ctx, chatId, nil, query.Ascending, 10)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			expected := &chat.Message{
				ChatId: chatId,

				MessageId: messageId,
				Data:      []byte("message"),

				IsSilent:      true,
				ContentLength: 3,

				Timestamp: time.Now(),
			}
			cloned := expected.Clone()

			require.NoError(t, s.PutMessage(ctx, expected))
			assert.True(t, expected.Id > 0)

			actual, err := s.GetMessageById(ctx, chatId, messageId)
			require.NoError(t, err)
			assertEquivalentMessageRecords(t, &cloned, actual)

			messages, err := s.GetAllMessagesByChat(ctx, chatId, nil, query.Ascending, 10)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			assertEquivalentMessageRecords(t, &cloned, messages[0])

			assert.Equal(t, chat.ErrMessageAlreadyExists, s.PutMessage(ctx, expected))
		}

		for i := 0; i < 3; i++ {
			chatId := chat.GetChatId("sender", fmt.Sprintf("receiver%d", i), true)

			require.NoError(t, s.DeleteMessage(ctx, chatId, messageId))

			_, err := s.GetMessageById(ctx, chatId, messageId)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			_, err = s.GetAllMessagesByChat(ctx, chatId, nil, query.Ascending, 10)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			if i == 0 {
				chatId := chat.GetChatId("sender", "receiver1", true)

				_, err := s.GetMessageById(ctx, chatId, messageId)
				require.NoError(t, err)

				_, err = s.GetAllMessagesByChat(ctx, chatId, nil, query.Ascending, 10)
				require.NoError(t, err)
			}

			require.NoError(t, s.DeleteMessage(ctx, chatId, messageId))
		}
	})
}

func testAdvancePointer(t *testing.T, s chat.Store) {
	t.Run("testAdvancePointer", func(t *testing.T) {
		ctx := context.Background()

		chatId := chat.GetChatId("user", "domain", true)

		assert.Equal(t, chat.ErrChatNotFound, s.AdvancePointer(ctx, chatId, "pointer"))

		record := &chat.Chat{
			ChatId:     chatId,
			ChatType:   chat.ChatTypeExternalApp,
			ChatTitle:  "domain",
			IsVerified: true,

			CodeUser: "user",

			CreatedAt: time.Now(),
		}
		require.NoError(t, s.PutChat(ctx, record))

		require.NoError(t, s.AdvancePointer(ctx, chatId, "pointer"))

		actual, err := s.GetChatById(ctx, chatId)
		require.NoError(t, err)
		require.NotNil(t, actual.ReadPointer)
		assert.Equal(t, "pointer", *actual.ReadPointer)
	})
}

func testGetUnreadCount(t *testing.T, s chat.Store) {
	t.Run("testGetUnreadCount", func(t *testing.T) {
		ctx := context.Background()

		chatId := chat.GetChatId("user", "domain", true)

		count, err := s.GetUnreadCount(ctx, chatId)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		chatRecord := &chat.Chat{
			ChatId:     chatId,
			ChatType:   chat.ChatTypeExternalApp,
			ChatTitle:  "domain",
			IsVerified: true,

			CodeUser: "user",

			CreatedAt: time.Now(),
		}
		require.NoError(t, s.PutChat(ctx, chatRecord))

		for i := 1; i <= 3; i++ {
			for j, isSilent := range []bool{false, true} {
				contentLength := uint8(i)
				if isSilent {
					contentLength *= 10
				}
				messageRecord := &chat.Message{
					ChatId: chatId,

					MessageId: fmt.Sprintf("message%d%d", i, j),
					Data:      []byte("message"),

					IsSilent:      isSilent,
					ContentLength: contentLength,

					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
				}
				require.NoError(t, s.PutMessage(ctx, messageRecord))
			}
		}

		count, err = s.GetUnreadCount(ctx, chatId)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)

		var deltaRead int
		for i := 1; i <= 3; i++ {
			deltaRead += 1

			for j := 0; j < 2; j++ {
				require.NoError(t, s.AdvancePointer(ctx, chatId, fmt.Sprintf("message%d%d", i, j)))

				count, err = s.GetUnreadCount(ctx, chatId)
				require.NoError(t, err)
				assert.EqualValues(t, 3-deltaRead, count)
			}
		}
	})
}

func testMuteState(t *testing.T, s chat.Store) {
	t.Run("testMuteState", func(t *testing.T) {
		ctx := context.Background()

		chatId := chat.GetChatId("user", "domain", true)

		require.NoError(t, s.PutChat(ctx, &chat.Chat{
			ChatId:     chatId,
			ChatType:   chat.ChatTypeExternalApp,
			ChatTitle:  "domain",
			IsVerified: true,

			CodeUser: "user",

			IsMuted: false,

			CreatedAt: time.Now(),
		}))

		for _, expected := range []bool{false, true, false} {
			require.NoError(t, s.SetMuteState(ctx, chatId, expected))

			actual, err := s.GetChatById(ctx, chatId)
			require.NoError(t, err)
			assert.Equal(t, expected, actual.IsMuted)
		}
	})
}

func tesSubscriptionState(t *testing.T, s chat.Store) {
	t.Run("tesSubscriptionState", func(t *testing.T) {
		ctx := context.Background()

		chatId := chat.GetChatId("user", "domain", true)

		require.NoError(t, s.PutChat(ctx, &chat.Chat{
			ChatId:     chatId,
			ChatType:   chat.ChatTypeExternalApp,
			ChatTitle:  "domain",
			IsVerified: true,

			CodeUser: "user",

			IsUnsubscribed: false,

			CreatedAt: time.Now(),
		}))

		for _, expected := range []bool{false, true, false} {
			require.NoError(t, s.SetSubscriptionState(ctx, chatId, expected))

			actual, err := s.GetChatById(ctx, chatId)
			require.NoError(t, err)
			assert.Equal(t, !expected, actual.IsUnsubscribed)
		}
	})
}

func testGetAllChatsByUserPaging(t *testing.T, s chat.Store) {
	t.Run("testGetAllChatsByUserPaging", func(t *testing.T) {
		ctx := context.Background()

		user := "user"

		var expected []*chat.Chat
		for i := 0; i < 10; i++ {
			merchant := fmt.Sprintf("merchant%d.com", i)

			record := &chat.Chat{
				ChatId:     chat.GetChatId(merchant, user, true),
				ChatType:   chat.ChatTypeExternalApp,
				ChatTitle:  merchant,
				IsVerified: true,
				CodeUser:   user,
				CreatedAt:  time.Now(),
			}
			require.NoError(t, s.PutChat(ctx, record))
			expected = append(expected, record)
		}

		actual, err := s.GetAllChatsForUser(ctx, user, query.ToCursor(0), query.Ascending, 100)
		require.NoError(t, err)
		require.Len(t, actual, len(expected))
		for i := 0; i < len(expected); i++ {
			assertEquivalentChatRecords(t, expected[i], actual[i])
		}

		actual, err = s.GetAllChatsForUser(ctx, user, query.ToCursor(1000), query.Descending, 100)
		require.NoError(t, err)
		require.Len(t, actual, len(expected))
		for i := 0; i < len(expected); i++ {
			assertEquivalentChatRecords(t, expected[len(expected)-i-1], actual[i])
		}

		actual, err = s.GetAllChatsForUser(ctx, user, query.ToCursor(expected[1].Id), query.Ascending, 3)
		require.NoError(t, err)
		require.Len(t, actual, 3)
		assertEquivalentChatRecords(t, expected[2], actual[0])
		assertEquivalentChatRecords(t, expected[3], actual[1])
		assertEquivalentChatRecords(t, expected[4], actual[2])

		actual, err = s.GetAllChatsForUser(ctx, user, query.ToCursor(expected[5].Id), query.Descending, 2)
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assertEquivalentChatRecords(t, expected[4], actual[0])
		assertEquivalentChatRecords(t, expected[3], actual[1])

		_, err = s.GetAllChatsForUser(ctx, user, query.ToCursor(1000), query.Ascending, 100)
		assert.Equal(t, chat.ErrChatNotFound, err)

		_, err = s.GetAllChatsForUser(ctx, user, query.ToCursor(0), query.Descending, 100)
		assert.Equal(t, chat.ErrChatNotFound, err)
	})
}

func testGetAllMessagesByChatPaging(t *testing.T, s chat.Store) {
	t.Run("testGetAllMessagesByChatPaging", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()

		for i, useSimilarTimestamps := range []bool{true, false} {
			chatId := chat.GetChatId(fmt.Sprintf("merchant%d", i), "user", true)

			var expected []*chat.Message
			for j := 0; j < 10; j++ {
				record := &chat.Message{
					ChatId: chatId,

					MessageId: base58.Encode([]byte(fmt.Sprintf("message%d", j))),
					Data:      []byte("data"),

					ContentLength: 1,

					Timestamp: start,
				}
				if !useSimilarTimestamps {
					record.Timestamp = start.Add(time.Duration(j) * time.Minute)
				}
				require.NoError(t, s.PutMessage(ctx, record))
				expected = append(expected, record)
			}

			actual, err := s.GetAllMessagesByChat(ctx, chatId, nil, query.Ascending, 100)
			require.NoError(t, err)
			require.Len(t, actual, len(expected))
			for i := 0; i < len(expected); i++ {
				assertEquivalentMessageRecords(t, expected[i], actual[i])
			}

			actual, err = s.GetAllMessagesByChat(ctx, chatId, nil, query.Descending, 100)
			require.NoError(t, err)
			require.Len(t, actual, len(expected))
			for i := 0; i < len(expected); i++ {
				assertEquivalentMessageRecords(t, expected[len(expected)-i-1], actual[i])
			}

			actual, err = s.GetAllMessagesByChat(ctx, chatId, getMessageCursor(t, expected[1]), query.Ascending, 3)
			require.NoError(t, err)
			require.Len(t, actual, 3)
			assertEquivalentMessageRecords(t, expected[2], actual[0])
			assertEquivalentMessageRecords(t, expected[3], actual[1])
			assertEquivalentMessageRecords(t, expected[4], actual[2])

			actual, err = s.GetAllMessagesByChat(ctx, chatId, getMessageCursor(t, expected[5]), query.Descending, 2)
			require.NoError(t, err)
			require.Len(t, actual, 2)
			assertEquivalentMessageRecords(t, expected[4], actual[0])
			assertEquivalentMessageRecords(t, expected[3], actual[1])

			_, err = s.GetAllMessagesByChat(ctx, chatId, getMessageCursor(t, expected[len(expected)-1]), query.Ascending, 100)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			_, err = s.GetAllMessagesByChat(ctx, chatId, getMessageCursor(t, expected[0]), query.Descending, 100)
			assert.Equal(t, chat.ErrMessageNotFound, err)

			_, err = s.GetAllMessagesByChat(ctx, chatId, []byte("does-not-exist"), query.Ascending, 100)
			assert.Equal(t, chat.ErrInvalidMessageCursor, err)
		}
	})
}

func assertEquivalentChatRecords(t *testing.T, obj1, obj2 *chat.Chat) {
	assert.Equal(t, obj1.ChatId, obj2.ChatId)
	assert.Equal(t, obj1.ChatType, obj2.ChatType)
	assert.Equal(t, obj1.ChatTitle, obj2.ChatTitle)
	assert.Equal(t, obj1.IsVerified, obj2.IsVerified)
	assert.Equal(t, obj1.CodeUser, obj2.CodeUser)
	assert.Equal(t, obj1.ReadPointer, obj2.ReadPointer)
	assert.Equal(t, obj1.IsMuted, obj2.IsMuted)
	assert.Equal(t, obj1.IsUnsubscribed, obj2.IsUnsubscribed)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}

func assertEquivalentMessageRecords(t *testing.T, obj1, obj2 *chat.Message) {
	assert.Equal(t, obj1.ChatId, obj2.ChatId)
	assert.Equal(t, obj1.MessageId, obj2.MessageId)
	assert.EqualValues(t, obj1.Data, obj2.Data)
	assert.Equal(t, obj1.IsSilent, obj2.IsSilent)
	assert.Equal(t, obj1.ContentLength, obj2.ContentLength)
	assert.Equal(t, obj1.Timestamp.Unix(), obj2.Timestamp.Unix())
}

func getMessageCursor(t *testing.T, record *chat.Message) query.Cursor {
	decoded, err := base58.Decode(record.MessageId)
	require.NoError(t, err)
	return query.Cursor(decoded)
}
