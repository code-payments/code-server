package memory

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

func TestInMemoryStore_GetChatMetadata(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}
	metadata := &chat.MetadataRecord{
		Id:        0,
		ChatId:    chatId,
		ChatType:  chat.ChatTypeTwoWay,
		CreatedAt: time.Now(),
		ChatTitle: pointer.String("hello"),
	}

	result, err := store.GetChatMetadata(context.Background(), chatId)
	require.ErrorIs(t, err, chat.ErrChatNotFound)
	require.Nil(t, result)

	require.NoError(t, store.PutChatV2(context.Background(), metadata))
	require.ErrorIs(t, store.PutChatV2(context.Background(), metadata), chat.ErrChatExists)

	result, err = store.GetChatMetadata(context.Background(), chatId)
	require.NoError(t, err)
	require.Equal(t, metadata.Clone(), result.Clone())
}

func TestInMemoryStore_GetAllChatsForUserV2(t *testing.T) {
	store := New()

	memberId := chat.MemberId("user123")

	chatIds, err := store.GetAllChatsForUserV2(context.Background(), memberId)
	require.NoError(t, err)
	require.Empty(t, chatIds)

	var expectedChatIds []chat.ChatId
	for i := 0; i < 10; i++ {
		chatId := chat.ChatId(bytes.Repeat([]byte{byte(i)}, 32))
		expectedChatIds = append(expectedChatIds, chatId)

		require.NoError(t, store.PutChatV2(context.Background(), &chat.MetadataRecord{
			Id:        0,
			ChatId:    chatId,
			ChatType:  chat.ChatTypeTwoWay,
			CreatedAt: time.Now(),
		}))

		require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
			ChatId:     chatId,
			MemberId:   memberId.String(),
			Platform:   chat.PlatformTwitter,
			PlatformId: "user",
			JoinedAt:   time.Now(),
		}))
		require.ErrorIs(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
			ChatId:     chatId,
			MemberId:   memberId.String(),
			Platform:   chat.PlatformTwitter,
			PlatformId: "user",
			JoinedAt:   time.Now(),
		}), chat.ErrMemberExists)
	}

	chatIds, err = store.GetAllChatsForUserV2(context.Background(), memberId)
	require.NoError(t, err)
	require.Equal(t, expectedChatIds, chatIds)
}

func TestInMemoryStore_GetAllChatsForUserV2_Pagination(t *testing.T) {
	store := New()

	memberId := chat.MemberId("user123")

	// Create 10 chats
	var chatIds []chat.ChatId
	for i := 0; i < 10; i++ {
		chatId := chat.ChatId(bytes.Repeat([]byte{byte(i)}, 32))
		chatIds = append(chatIds, chatId)

		require.NoError(t, store.PutChatV2(context.Background(), &chat.MetadataRecord{
			ChatId:    chatId,
			ChatType:  chat.ChatTypeTwoWay,
			CreatedAt: time.Now(),
		}))

		require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
			ChatId:     chatId,
			MemberId:   memberId.String(),
			Platform:   chat.PlatformTwitter,
			PlatformId: "user",
			JoinedAt:   time.Now(),
		}))
	}

	reversedChatIds := slices.Clone(chatIds)
	slices.Reverse(reversedChatIds)

	t.Run("Ascending Order", func(t *testing.T) {
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithDirection(query.Ascending))
		require.NoError(t, err)
		require.Equal(t, chatIds, result)
	})

	t.Run("Descending Order", func(t *testing.T) {
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithDirection(query.Descending))
		require.NoError(t, err)
		require.Equal(t, reversedChatIds, result)
	})

	t.Run("With Cursor", func(t *testing.T) {
		cursor := chatIds[3][:]
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithDirection(query.Ascending), query.WithCursor(cursor))
		require.NoError(t, err)
		require.Equal(t, chatIds[4:], result)
	})

	t.Run("With Cursor (Descending)", func(t *testing.T) {
		cursor := reversedChatIds[6][:]
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithDirection(query.Descending), query.WithCursor(cursor))
		require.NoError(t, err)
		require.Equal(t, reversedChatIds[7:], result)
	})

	t.Run("With Limit", func(t *testing.T) {
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithLimit(5))
		require.NoError(t, err)
		require.Equal(t, chatIds[:5], result)
	})

	t.Run("With Limit (Descending)", func(t *testing.T) {
		cursor := reversedChatIds[4][:]
		result, err := store.GetAllChatsForUserV2(context.Background(), memberId, query.WithDirection(query.Descending), query.WithCursor(cursor), query.WithLimit(3))
		require.NoError(t, err)
		require.Equal(t, reversedChatIds[5:8], result)
	})
}

func TestInMemoryStore_GetChatMessageV2(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}
	messageId := chat.GenerateMessageId()
	message := &chat.MessageRecord{
		ChatId:    chatId,
		MessageId: messageId,
		Payload:   []byte("payload"),
	}

	err := store.PutChatMessageV2(context.Background(), message)
	require.NoError(t, err)

	result, err := store.GetChatMessageV2(context.Background(), chatId, messageId)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, bytes.Equal(result.MessageId[:], messageId[:]))
}

// TODO: Need proper pagination tests
func TestInMemoryStore_GetAllChatMessagesV2(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}

	var expectedMessages []*chat.MessageRecord
	for i := 0; i < 10; i++ {
		message := &chat.MessageRecord{
			ChatId:    chatId,
			MessageId: chat.GenerateMessageId(),
			Payload:   []byte(fmt.Sprintf("payload-%d", i)),
		}
		expectedMessages = append(expectedMessages, message)

		// TODO: We might need a way to address this longer term.
		time.Sleep(time.Millisecond)

		require.NoError(t, store.PutChatMessageV2(context.Background(), message))
		require.ErrorIs(t, store.PutChatMessageV2(context.Background(), message), chat.ErrMessageExists)
	}

	isSorted := slices.IsSortedFunc(expectedMessages, func(a, b *chat.MessageRecord) int {
		return bytes.Compare(a.MessageId[:], b.MessageId[:])
	})
	require.True(t, isSorted)

	messages, err := store.GetAllChatMessagesV2(context.Background(), chatId)
	require.NoError(t, err)
	require.Equal(t, len(expectedMessages), len(messages))

	for i := 0; i < len(messages); i++ {
		require.Equal(t, expectedMessages[i].ChatId, messages[i].ChatId)
		require.Equal(t, expectedMessages[i].MessageId, messages[i].MessageId)
		require.Equal(t, expectedMessages[i].Sender, messages[i].Sender)
		require.Equal(t, expectedMessages[i].Payload, messages[i].Payload)
		require.Equal(t, expectedMessages[i].IsSilent, messages[i].IsSilent)
	}
}

func TestInMemoryStore_GetAllChatMessagesV2_Pagination(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}

	var expectedMessages []*chat.MessageRecord
	for i := 0; i < 10; i++ {
		message := &chat.MessageRecord{
			ChatId:    chatId,
			MessageId: chat.GenerateMessageId(),
			Payload:   []byte(fmt.Sprintf("payload-%d", i)),
		}
		expectedMessages = append(expectedMessages, message)
		time.Sleep(time.Millisecond)
		require.NoError(t, store.PutChatMessageV2(context.Background(), message))
	}

	reversedMessages := slices.Clone(expectedMessages)
	slices.Reverse(reversedMessages)

	t.Run("Ascending order", func(t *testing.T) {
		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId, query.WithDirection(query.Ascending))
		require.NoError(t, err)
		require.Equal(t, expectedMessages, messages)
	})

	t.Run("Descending order", func(t *testing.T) {
		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId, query.WithDirection(query.Descending))
		require.NoError(t, err)
		require.Equal(t, reversedMessages, messages)
	})

	t.Run("With limit", func(t *testing.T) {
		limit := uint64(5)
		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId, query.WithDirection(query.Ascending), query.WithLimit(limit))
		require.NoError(t, err)
		require.Equal(t, expectedMessages[:limit], messages)
	})

	t.Run("With cursor", func(t *testing.T) {
		cursor := expectedMessages[3].MessageId[:]
		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId, query.WithDirection(query.Ascending), query.WithCursor(cursor))
		require.NoError(t, err)
		require.Equal(t, expectedMessages[4:], messages)
	})

	t.Run("With cursor and limit", func(t *testing.T) {
		cursor := reversedMessages[3].MessageId[:]
		limit := uint64(3)
		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId, query.WithDirection(query.Descending), query.WithCursor(cursor), query.WithLimit(limit))
		require.NoError(t, err)
		require.Equal(t, reversedMessages[4:7], messages)
	})
}

// TODO: Need proper pagination tests
func TestInMemoryStore_GetChatMembersV2(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}

	var expectedMembers []*chat.MemberRecord
	for i := 0; i < 10; i++ {
		member := &chat.MemberRecord{
			ChatId:     chatId,
			MemberId:   fmt.Sprintf("user%d", i),
			Owner:      fmt.Sprintf("owner%d", i),
			Platform:   chat.PlatformTwitter,
			PlatformId: fmt.Sprintf("twitter%d", i),
			IsMuted:    true,
			JoinedAt:   time.Now(),
		}

		dPtr := chat.GenerateMessageId()
		time.Sleep(time.Millisecond)
		rPtr := chat.GenerateMessageId()

		member.DeliveryPointer = &dPtr
		member.ReadPointer = &rPtr

		expectedMembers = append(expectedMembers, member)

		require.NoError(t, store.PutChatMemberV2(context.Background(), member))
		require.ErrorIs(t, store.PutChatMemberV2(context.Background(), member), chat.ErrMemberExists)
	}

	members, err := store.GetChatMembersV2(context.Background(), chatId)
	require.NoError(t, err)
	require.Equal(t, expectedMembers, members)
}

func TestInMemoryStore_IsChatMember(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}
	memberId := chat.MemberId("user123")

	isMember, err := store.IsChatMember(context.Background(), chatId, memberId)
	require.NoError(t, err)
	require.False(t, isMember)

	require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
		ChatId:     chatId,
		MemberId:   memberId.String(),
		Platform:   chat.PlatformTwitter,
		PlatformId: "user",
		JoinedAt:   time.Now(),
	}))

	isMember, err = store.IsChatMember(context.Background(), chatId, memberId)
	require.NoError(t, err)
	require.True(t, isMember)
}

func TestInMemoryStore_PutChatV2(t *testing.T) {
	store := New()

	for i, expected := range []*chat.MetadataRecord{
		{
			ChatType:  chat.ChatTypeTwoWay,
			CreatedAt: time.Now(),
		},
		{
			ChatType:  chat.ChatTypeTwoWay,
			CreatedAt: time.Now(),
			ChatTitle: pointer.String("hello"),
		},
	} {
		expected.ChatId = chat.ChatId(bytes.Repeat([]byte{byte(i)}, 32))

		require.NoError(t, store.PutChatV2(context.Background(), expected))

		other := expected.Clone()
		other.ChatTitle = pointer.String("mutated")
		require.ErrorIs(t, store.PutChatV2(context.Background(), &other), chat.ErrChatExists)

		actual, err := store.GetChatMetadata(context.Background(), expected.ChatId)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}

	for _, invalid := range []*chat.MetadataRecord{
		{},
		{
			ChatId: chat.ChatId{1, 2, 3},
		},
		{
			ChatId:    chat.ChatId{1, 2, 3},
			CreatedAt: time.Now(),
		},
		{
			ChatId:   chat.ChatId{1, 2, 3},
			ChatType: chat.ChatTypeTwoWay,
		},
	} {
		require.Error(t, store.PutChatV2(context.Background(), invalid))
	}
}

func TestInMemoryStore_SetChatMuteStateV2(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}
	memberId := chat.MemberId("user123")

	require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
		ChatId:     chatId,
		Platform:   chat.PlatformTwitter,
		PlatformId: "user",
		MemberId:   memberId.String(),
		JoinedAt:   time.Now(),
	}))

	members, err := store.GetChatMembersV2(context.Background(), chatId)
	require.NoError(t, err)
	require.False(t, members[0].IsMuted)

	require.NoError(t, store.SetChatMuteStateV2(context.Background(), chatId, memberId, true))

	members, err = store.GetChatMembersV2(context.Background(), chatId)
	require.NoError(t, err)
	require.True(t, members[0].IsMuted)
}

func TestInMemoryStore_GetChatUnreadCountV2(t *testing.T) {
	store := New()

	// Create multiple chats
	chats := []chat.ChatId{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
	}
	counts := []int{0, 0, 0}

	ourMemberId := chat.MemberId("our_user")
	otherMemberId := chat.MemberId("other_user")

	for chatIdx, chatId := range chats {
		// Add members to the chat
		require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
			ChatId:     chatId,
			Platform:   chat.PlatformTwitter,
			PlatformId: "our_user",
			MemberId:   ourMemberId.String(),
			JoinedAt:   time.Now(),
		}))
		require.NoError(t, store.PutChatMemberV2(context.Background(), &chat.MemberRecord{
			ChatId:     chatId,
			Platform:   chat.PlatformTwitter,
			PlatformId: "other_user",
			MemberId:   otherMemberId.String(),
			JoinedAt:   time.Now(),
		}))

		// Generate N messages for each chat
		N := 10
		for i := 0; i < N; i++ {
			sender := ourMemberId
			if rand.IntN(100) < 50 { // Approximately 50% chance for a message to be from the other user
				sender = otherMemberId
				counts[chatIdx]++
			}

			require.NoError(t, store.PutChatMessageV2(context.Background(), &chat.MessageRecord{
				ChatId:    chatId,
				MessageId: chat.GenerateMessageId(),
				Sender:    &sender,
				Payload:   []byte(fmt.Sprintf("Message %d for chat %v", i, chatId)),
			}))

			time.Sleep(time.Millisecond)
		}
	}

	// Verify that each chat has a distinct unread count
	for chatIdx, chatId := range chats {
		ptr := chat.GenerateMessageIdAtTime(time.Now().Add(-time.Hour))
		count, err := store.GetChatUnreadCountV2(context.Background(), chatId, ourMemberId, &ptr)
		require.NoError(t, err)
		require.EqualValues(t, counts[chatIdx], count)

		if count == 0 {
			continue
		}

		messages, err := store.GetAllChatMessagesV2(context.Background(), chatId)
		require.NoError(t, err)

		var offset *chat.MessageId
		for _, message := range messages {
			if message.Sender != nil && !bytes.Equal(*message.Sender, ourMemberId) {
				offset = &message.MessageId
				break
			}
		}
		require.NotNil(t, offset)

		newCount, err := store.GetChatUnreadCountV2(context.Background(), chatId, ourMemberId, offset)
		require.NoError(t, err)
		require.Equal(t, count-1, newCount)
	}
}

func TestInMemoryStore_AdvanceChatPointerV2(t *testing.T) {
	store := New()

	chatId := chat.ChatId{1, 2, 3}
	memberId := chat.MemberId("user123")

	// Create a chat and add a member
	metadata := &chat.MetadataRecord{
		Id:        0,
		ChatId:    chatId,
		ChatType:  chat.ChatTypeTwoWay,
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.PutChatV2(context.Background(), metadata))

	member := &chat.MemberRecord{
		ChatId:     chatId,
		MemberId:   memberId.String(),
		Platform:   chat.PlatformTwitter,
		PlatformId: "user",
		JoinedAt:   time.Now(),

		DeliveryPointer: nil,
		ReadPointer:     nil,
	}
	require.NoError(t, store.PutChatMemberV2(context.Background(), member))

	// Test advancing delivery pointer
	message1 := chat.GenerateMessageId()
	advanced, err := store.AdvanceChatPointerV2(context.Background(), chatId, memberId, chat.PointerTypeDelivered, message1)
	require.NoError(t, err)
	require.True(t, advanced)

	// Test advancing read pointer
	message2 := chat.GenerateMessageId()
	advanced, err = store.AdvanceChatPointerV2(context.Background(), chatId, memberId, chat.PointerTypeRead, message2)
	require.NoError(t, err)
	require.True(t, advanced)

	// Test advancing to an earlier message (should not advance)
	advanced, err = store.AdvanceChatPointerV2(context.Background(), chatId, memberId, chat.PointerTypeDelivered, message1)
	require.NoError(t, err)
	require.False(t, advanced)

	// Test with invalid pointer type
	_, err = store.AdvanceChatPointerV2(context.Background(), chatId, memberId, chat.PointerType(8), message2)
	require.ErrorIs(t, err, chat.ErrInvalidPointerType)

	// Test with non-existent chat
	nonExistentChatId := chat.ChatId{4, 5, 6}
	_, err = store.AdvanceChatPointerV2(context.Background(), nonExistentChatId, memberId, chat.PointerTypeDelivered, message2)
	require.ErrorIs(t, err, chat.ErrMemberNotFound)

	// Test with non-existent member
	nonExistentMemberId := chat.MemberId("nonexistent")
	_, err = store.AdvanceChatPointerV2(context.Background(), chatId, nonExistentMemberId, chat.PointerTypeDelivered, message2)
	require.ErrorIs(t, err, chat.ErrMemberNotFound)
}
