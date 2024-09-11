package chat

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/badgecount"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestSendChatMessage_HappyPath(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), true)

	var expectedBadgeCount int
	for i := 0; i < 10; i++ {
		chatMessage := newRandomChatMessage(t, i+1)
		expectedBadgeCount += 1

		canPush, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, true, receiver, chatMessage, false)
		require.NoError(t, err)

		assert.True(t, canPush)

		assert.NotNil(t, chatMessage.MessageId)
		assert.NotNil(t, chatMessage.Ts)
		assert.Nil(t, chatMessage.Cursor)

		env.assertChatRecordSaved(t, chatTitle, receiver, true)
		env.assertChatMessageRecordSaved(t, chatId, chatMessage, false)
		env.assertBadgeCount(t, receiver, expectedBadgeCount)
	}
}

func TestSendChatMessage_VerifiedChat(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)

	for _, isVerified := range []bool{true, false} {
		chatMessage := newRandomChatMessage(t, 1)
		_, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, isVerified, receiver, chatMessage, true)
		require.NoError(t, err)
		env.assertChatRecordSaved(t, chatTitle, receiver, isVerified)
	}
}

func TestSendChatMessage_SilentMessage(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), true)

	for i, isSilent := range []bool{true, false} {
		chatMessage := newRandomChatMessage(t, 1)
		canPush, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, true, receiver, chatMessage, isSilent)
		require.NoError(t, err)
		assert.Equal(t, !isSilent, canPush)
		env.assertChatMessageRecordSaved(t, chatId, chatMessage, isSilent)
		env.assertBadgeCount(t, receiver, i)
	}
}

func TestSendChatMessage_MuteState(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), true)

	for _, isMuted := range []bool{false, true} {
		if isMuted {
			env.muteChat(t, chatId)
		}

		chatMessage := newRandomChatMessage(t, 1)
		canPush, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, true, receiver, chatMessage, false)
		require.NoError(t, err)
		assert.Equal(t, !isMuted, canPush)
		env.assertChatMessageRecordSaved(t, chatId, chatMessage, false)
		env.assertBadgeCount(t, receiver, 1)
	}
}

func TestSendChatMessage_SubscriptionState(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), true)

	for _, isUnsubscribed := range []bool{false, true} {
		if isUnsubscribed {
			env.unsubscribeFromChat(t, chatId)
		}

		chatMessage := newRandomChatMessage(t, 1)
		canPush, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, true, receiver, chatMessage, false)
		require.NoError(t, err)
		assert.Equal(t, !isUnsubscribed, canPush)
		if isUnsubscribed {
			env.assertChatMessageRecordNotSaved(t, chatId, chatMessage.MessageId)
		} else {
			env.assertChatMessageRecordSaved(t, chatId, chatMessage, false)
		}
		env.assertBadgeCount(t, receiver, 1)
	}
}

func TestSendChatMessage_InvalidProtoMessage(t *testing.T) {
	env := setup(t)

	chatTitle := CodeTeamName
	receiver := testutil.NewRandomAccount(t)
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), true)

	chatMessage := newRandomChatMessage(t, 1)
	chatMessage.Content = nil

	canPush, err := SendChatMessage(env.ctx, env.data, chatTitle, chat_v1.ChatTypeInternal, true, receiver, chatMessage, false)
	assert.Error(t, err)
	assert.False(t, canPush)
	env.assertChatRecordNotSaved(t, chatId)
	env.assertChatMessageRecordNotSaved(t, chatId, chatMessage.MessageId)
	env.assertBadgeCount(t, receiver, 0)
}

type testEnv struct {
	ctx  context.Context
	data code_data.Provider
}

func setup(t *testing.T) *testEnv {
	return &testEnv{
		ctx:  context.Background(),
		data: code_data.NewTestDataProvider(),
	}
}

func newRandomChatMessage(t *testing.T, contentLength int) *chatpb.ChatMessage {
	var content []*chatpb.Content
	for i := 0; i < contentLength; i++ {
		content = append(content, &chatpb.Content{
			Type: &chatpb.Content_ServerLocalized{
				ServerLocalized: &chatpb.ServerLocalizedContent{
					KeyOrText: fmt.Sprintf("key%d", rand.Uint32()),
				},
			},
		})
	}

	msg, err := newProtoChatMessage(testutil.NewRandomAccount(t).PrivateKey().ToBase58(), content, time.Now())
	require.NoError(t, err)
	return msg
}

func (e *testEnv) assertChatRecordSaved(t *testing.T, chatTitle string, receiver *common.Account, isVerified bool) {
	chatId := chat_v1.GetChatId(chatTitle, receiver.PublicKey().ToBase58(), isVerified)

	chatRecord, err := e.data.GetChatByIdV1(e.ctx, chatId)
	require.NoError(t, err)

	assert.Equal(t, chatId[:], chatRecord.ChatId[:])
	assert.Equal(t, chat_v1.ChatTypeInternal, chatRecord.ChatType)
	assert.Equal(t, chatTitle, chatRecord.ChatTitle)
	assert.Equal(t, isVerified, chatRecord.IsVerified)
	assert.Equal(t, receiver.PublicKey().ToBase58(), chatRecord.CodeUser)
	assert.Nil(t, chatRecord.ReadPointer)
	assert.False(t, chatRecord.IsMuted)
	assert.False(t, chatRecord.IsUnsubscribed)
}

func (e *testEnv) assertChatMessageRecordSaved(t *testing.T, chatId chat_v1.ChatId, protoMessage *chatpb.ChatMessage, isSilent bool) {
	messageRecord, err := e.data.GetChatMessageV1(e.ctx, chatId, base58.Encode(protoMessage.GetMessageId().Value))
	require.NoError(t, err)

	cloned := proto.Clone(protoMessage).(*chatpb.ChatMessage)
	cloned.MessageId = nil
	cloned.Ts = nil
	cloned.Cursor = nil

	expectedData, err := proto.Marshal(cloned)
	require.NoError(t, err)

	assert.Equal(t, messageRecord.MessageId, base58.Encode(protoMessage.GetMessageId().Value))
	assert.Equal(t, chatId[:], messageRecord.ChatId[:])
	assert.Equal(t, expectedData, messageRecord.Data)
	assert.Equal(t, isSilent, messageRecord.IsSilent)
	assert.EqualValues(t, messageRecord.ContentLength, len(protoMessage.Content))
	assert.Equal(t, messageRecord.Timestamp.Unix(), protoMessage.Ts.Seconds)
}

func (e *testEnv) assertBadgeCount(t *testing.T, owner *common.Account, expected int) {
	badgeCountRecord, err := e.data.GetBadgeCount(e.ctx, owner.PublicKey().ToBase58())
	if err == badgecount.ErrBadgeCountNotFound {
		assert.Equal(t, 0, expected)
		return
	}
	require.NoError(t, err)
	assert.EqualValues(t, expected, badgeCountRecord.BadgeCount)
}

func (e *testEnv) assertChatRecordNotSaved(t *testing.T, chatId chat_v1.ChatId) {
	_, err := e.data.GetChatByIdV1(e.ctx, chatId)
	assert.Equal(t, chat_v1.ErrChatNotFound, err)

}

func (e *testEnv) assertChatMessageRecordNotSaved(t *testing.T, chatId chat_v1.ChatId, messageId *chatpb.ChatMessageId) {
	_, err := e.data.GetChatMessageV1(e.ctx, chatId, base58.Encode(messageId.Value))
	assert.Equal(t, chat_v1.ErrMessageNotFound, err)

}

func (e *testEnv) muteChat(t *testing.T, chatId chat_v1.ChatId) {
	require.NoError(t, e.data.SetChatMuteStateV1(e.ctx, chatId, true))
}

func (e *testEnv) unsubscribeFromChat(t *testing.T, chatId chat_v1.ChatId) {
	require.NoError(t, e.data.SetChatSubscriptionStateV1(e.ctx, chatId, false))
}
