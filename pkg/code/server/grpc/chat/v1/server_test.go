package chat_v1

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	"github.com/code-payments/code-server/pkg/code/localization"
	"github.com/code-payments/code-server/pkg/kin"
	memory_push "github.com/code-payments/code-server/pkg/push/memory"
	"github.com/code-payments/code-server/pkg/testutil"
)

// todo: This could use a refactor with some testing utilities

func TestGetChatsAndMessages_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	localization.LoadTestKeys(map[language.Tag]map[string]string{
		language.English: {
			localization.ChatTitleCodeTeam: "Code Team",
			"msg.body.key":                 "localized message body content",
		},
	})
	defer localization.ResetKeys()

	testExternalAppDomain := "test.com"

	cashTransactionsChatId := chat.GetChatId(chat_util.CashTransactionsName, owner.PublicKey().ToBase58(), true)
	codeTeamChatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)
	verifiedExternalAppChatId := chat.GetChatId(testExternalAppDomain, owner.PublicKey().ToBase58(), true)
	unverifiedExternalAppChatId := chat.GetChatId(testExternalAppDomain, owner.PublicKey().ToBase58(), false)

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

	getCodeTeamMessagesReq := &chatpb.GetMessagesRequest{
		ChatId: codeTeamChatId.ToProto(),
		Owner:  owner.ToProto(),
	}
	getCodeTeamMessagesReq.Signature = signProtoMessage(t, getCodeTeamMessagesReq, owner, false)

	getCashTransactionsMessagesReq := &chatpb.GetMessagesRequest{
		ChatId: cashTransactionsChatId.ToProto(),
		Owner:  owner.ToProto(),
	}
	getCashTransactionsMessagesReq.Signature = signProtoMessage(t, getCashTransactionsMessagesReq, owner, false)

	getVerifiedExternalAppMessagesReq := &chatpb.GetMessagesRequest{
		ChatId: verifiedExternalAppChatId.ToProto(),
		Owner:  owner.ToProto(),
	}
	getVerifiedExternalAppMessagesReq.Signature = signProtoMessage(t, getVerifiedExternalAppMessagesReq, owner, false)

	getUnverifiedExternalAppMessagesReq := &chatpb.GetMessagesRequest{
		ChatId: unverifiedExternalAppChatId.ToProto(),
		Owner:  owner.ToProto(),
	}
	getUnverifiedExternalAppMessagesReq.Signature = signProtoMessage(t, getUnverifiedExternalAppMessagesReq, owner, false)

	getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_NOT_FOUND, getChatsResp.Result)
	assert.Empty(t, getChatsResp.Chats)

	getMessagesResp, err := env.client.GetMessages(env.ctx, getCodeTeamMessagesReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetMessagesResponse_NOT_FOUND, getMessagesResp.Result)
	assert.Empty(t, getMessagesResp.Messages)

	expectedCodeTeamMessage := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						KeyOrText: "msg.body.key",
					},
				},
			},
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: chatpb.ExchangeDataContent_RECEIVED,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: &transactionpb.ExchangeData{
								Currency:     "usd",
								ExchangeRate: 0.1,
								NativeAmount: 1.00,
								Quarks:       kin.ToQuarks(10),
							},
						},
					},
				},
			},
		},
	}
	env.sendInternalChatMessage(t, expectedCodeTeamMessage, chat_util.CodeTeamName, owner)

	expectedCashTransactionsMessage := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: chatpb.ExchangeDataContent_GAVE,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: &transactionpb.ExchangeData{
								Currency:     "usd",
								ExchangeRate: 0.1,
								NativeAmount: 4.2,
								Quarks:       kin.ToQuarks(42),
							},
						},
					},
				},
			},
		},
	}
	env.sendInternalChatMessage(t, expectedCashTransactionsMessage, chat_util.CashTransactionsName, owner)

	expectedVerifiedExternalAppMessage := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    testutil.NewRandomAccount(t).ToProto(),
						Nonce:            make([]byte, 24),
						EncryptedPayload: []byte("verified message secret"),
					},
				},
			},
		},
	}
	env.sendExternalAppChatMessage(t, expectedVerifiedExternalAppMessage, testExternalAppDomain, true, owner)

	// Technically this type of message would never be allowed for an unverified chat
	expectedUnverifiedExternalAppMessage := &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    testutil.NewRandomAccount(t).ToProto(),
						Nonce:            make([]byte, 24),
						EncryptedPayload: []byte("unverified message secret"),
					},
				},
			},
		},
	}
	env.sendExternalAppChatMessage(t, expectedUnverifiedExternalAppMessage, testExternalAppDomain, false, owner)

	getChatsResp, err = env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
	require.Len(t, getChatsResp.Chats, 4)

	assert.Equal(t, codeTeamChatId[:], getChatsResp.Chats[0].ChatId.Value)
	assert.Equal(t, "Code Team", getChatsResp.Chats[0].GetLocalized().KeyOrText)
	assert.Nil(t, getChatsResp.Chats[0].ReadPointer)
	assert.EqualValues(t, 1, getChatsResp.Chats[0].NumUnread)
	assert.False(t, getChatsResp.Chats[0].IsMuted)
	assert.True(t, getChatsResp.Chats[0].IsSubscribed)
	assert.True(t, getChatsResp.Chats[0].CanMute)
	assert.False(t, getChatsResp.Chats[0].CanUnsubscribe)
	assert.True(t, getChatsResp.Chats[0].IsVerified)

	assert.Equal(t, cashTransactionsChatId[:], getChatsResp.Chats[1].ChatId.Value)
	assert.Equal(t, localization.ChatTitleCashTransactions, getChatsResp.Chats[1].GetLocalized().KeyOrText)
	assert.Nil(t, getChatsResp.Chats[1].ReadPointer)
	assert.EqualValues(t, 0, getChatsResp.Chats[1].NumUnread)
	assert.False(t, getChatsResp.Chats[1].IsMuted)
	assert.True(t, getChatsResp.Chats[1].IsSubscribed)
	assert.True(t, getChatsResp.Chats[1].CanMute)
	assert.False(t, getChatsResp.Chats[1].CanUnsubscribe)
	assert.True(t, getChatsResp.Chats[1].IsVerified)

	assert.Equal(t, verifiedExternalAppChatId[:], getChatsResp.Chats[2].ChatId.Value)
	assert.Equal(t, testExternalAppDomain, getChatsResp.Chats[2].GetDomain().Value)
	assert.Nil(t, getChatsResp.Chats[2].ReadPointer)
	assert.EqualValues(t, 1, getChatsResp.Chats[2].NumUnread)
	assert.False(t, getChatsResp.Chats[2].IsMuted)
	assert.True(t, getChatsResp.Chats[2].IsSubscribed)
	assert.True(t, getChatsResp.Chats[2].CanMute)
	assert.True(t, getChatsResp.Chats[2].CanUnsubscribe)
	assert.True(t, getChatsResp.Chats[2].IsVerified)

	assert.Equal(t, unverifiedExternalAppChatId[:], getChatsResp.Chats[3].ChatId.Value)
	assert.Equal(t, testExternalAppDomain, getChatsResp.Chats[3].GetDomain().Value)
	assert.Nil(t, getChatsResp.Chats[3].ReadPointer)
	assert.EqualValues(t, 0, getChatsResp.Chats[3].NumUnread)
	assert.False(t, getChatsResp.Chats[3].IsMuted)
	assert.True(t, getChatsResp.Chats[3].IsSubscribed)
	assert.True(t, getChatsResp.Chats[3].CanMute)
	assert.True(t, getChatsResp.Chats[3].CanUnsubscribe)
	assert.False(t, getChatsResp.Chats[3].IsVerified)

	getMessagesResp, err = env.client.GetMessages(env.ctx, getCodeTeamMessagesReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetMessagesResponse_OK, getMessagesResp.Result)
	require.Len(t, getMessagesResp.Messages, 1)
	assert.Equal(t, expectedCodeTeamMessage.MessageId.Value, getMessagesResp.Messages[0].Cursor.Value)
	getMessagesResp.Messages[0].Cursor = nil
	expectedCodeTeamMessage.Content[0].GetServerLocalized().KeyOrText = "localized message body content"
	assert.True(t, proto.Equal(expectedCodeTeamMessage, getMessagesResp.Messages[0]))

	getMessagesResp, err = env.client.GetMessages(env.ctx, getCashTransactionsMessagesReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetMessagesResponse_OK, getMessagesResp.Result)
	require.Len(t, getMessagesResp.Messages, 1)
	assert.Equal(t, expectedCashTransactionsMessage.MessageId.Value, getMessagesResp.Messages[0].Cursor.Value)
	getMessagesResp.Messages[0].Cursor = nil
	assert.True(t, proto.Equal(expectedCashTransactionsMessage, getMessagesResp.Messages[0]))

	getMessagesResp, err = env.client.GetMessages(env.ctx, getVerifiedExternalAppMessagesReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetMessagesResponse_OK, getMessagesResp.Result)
	require.Len(t, getMessagesResp.Messages, 1)
	assert.Equal(t, expectedVerifiedExternalAppMessage.MessageId.Value, getMessagesResp.Messages[0].Cursor.Value)
	getMessagesResp.Messages[0].Cursor = nil
	assert.True(t, proto.Equal(expectedVerifiedExternalAppMessage, getMessagesResp.Messages[0]))

	getMessagesResp, err = env.client.GetMessages(env.ctx, getUnverifiedExternalAppMessagesReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetMessagesResponse_OK, getMessagesResp.Result)
	require.Len(t, getMessagesResp.Messages, 1)
	assert.Equal(t, expectedUnverifiedExternalAppMessage.MessageId.Value, getMessagesResp.Messages[0].Cursor.Value)
	getMessagesResp.Messages[0].Cursor = nil
	assert.True(t, proto.Equal(expectedUnverifiedExternalAppMessage, getMessagesResp.Messages[0]))
}

func TestChatHistoryReadState_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)

	var messageIds []*chatpb.ChatMessageId
	for i := 0; i < 5; i++ {
		message := &chatpb.ChatMessage{
			MessageId: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
			Ts: timestamppb.Now(),
			Content: []*chatpb.Content{
				{
					Type: &chatpb.Content_ServerLocalized{
						ServerLocalized: &chatpb.ServerLocalizedContent{
							KeyOrText: fmt.Sprintf("msg.body.key%d", i),
						},
					},
				},
			},
		}
		messageIds = append(messageIds, message.MessageId)
		env.sendInternalChatMessage(t, message, chat_util.CodeTeamName, owner)
	}

	advancePointerReq := &chatpb.AdvancePointerRequest{
		Owner:  owner.ToProto(),
		ChatId: chatId.ToProto(),
		Pointer: &chatpb.Pointer{
			Kind:  chatpb.Pointer_READ,
			Value: messageIds[len(messageIds)/2],
		},
	}
	advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, owner, false)

	advancePointerResp, err := env.client.AdvancePointer(env.ctx, advancePointerReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.AdvancePointerResponse_OK, advancePointerResp.Result)

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

	getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
	require.Len(t, getChatsResp.Chats, 1)
	assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
	require.NotNil(t, getChatsResp.Chats[0].ReadPointer)
	assert.Equal(t, messageIds[len(messageIds)/2].Value, getChatsResp.Chats[0].ReadPointer.Value.Value)
}

func TestChatHistoryReadState_NegativeProgress(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)

	var messageIds []*chatpb.ChatMessageId
	for i := 0; i < 5; i++ {
		message := &chatpb.ChatMessage{
			MessageId: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
			Ts: timestamppb.Now(),
			Content: []*chatpb.Content{
				{
					Type: &chatpb.Content_ServerLocalized{
						ServerLocalized: &chatpb.ServerLocalizedContent{
							KeyOrText: fmt.Sprintf("msg.body.key%d", i),
						},
					},
				},
			},
		}
		messageIds = append(messageIds, message.MessageId)
		env.sendInternalChatMessage(t, message, chat_util.CodeTeamName, owner)
	}

	for i := len(messageIds) - 1; i >= 0; i-- {
		advancePointerReq := &chatpb.AdvancePointerRequest{
			Owner:  owner.ToProto(),
			ChatId: chatId.ToProto(),
			Pointer: &chatpb.Pointer{
				Kind:  chatpb.Pointer_READ,
				Value: messageIds[i],
			},
		}
		advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, owner, false)

		advancePointerResp, err := env.client.AdvancePointer(env.ctx, advancePointerReq)
		require.NoError(t, err)
		assert.Equal(t, chatpb.AdvancePointerResponse_OK, advancePointerResp.Result)
	}

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

	getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
	require.Len(t, getChatsResp.Chats, 1)
	assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
	require.NotNil(t, getChatsResp.Chats[0].ReadPointer)
	assert.Equal(t, messageIds[len(messageIds)-1].Value, getChatsResp.Chats[0].ReadPointer.Value.Value)
}

func TestChatHistoryReadState_ChatNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)

	advancePointerReq := &chatpb.AdvancePointerRequest{
		Owner:  owner.ToProto(),
		ChatId: chatId.ToProto(),
		Pointer: &chatpb.Pointer{
			Kind: chatpb.Pointer_READ,
			Value: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
		},
	}
	advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, owner, false)

	advancePointerResp, err := env.client.AdvancePointer(env.ctx, advancePointerReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.AdvancePointerResponse_CHAT_NOT_FOUND, advancePointerResp.Result)
}

func TestChatHistoryReadState_MessageNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)

	chatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)

	env.sendInternalChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						KeyOrText: "msg.body.key",
					},
				},
			},
		},
	}, chat_util.CodeTeamName, owner)

	advancePointerReq := &chatpb.AdvancePointerRequest{
		Owner:  owner.ToProto(),
		ChatId: chatId.ToProto(),
		Pointer: &chatpb.Pointer{
			Kind: chatpb.Pointer_READ,
			Value: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
		},
	}
	advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, owner, false)

	advancePointerResp, err := env.client.AdvancePointer(env.ctx, advancePointerReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.AdvancePointerResponse_MESSAGE_NOT_FOUND, advancePointerResp.Result)
}

func TestChatMuteState_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	testExternalAppDomain := "test.com"

	chatId := chat.GetChatId(testExternalAppDomain, owner.PublicKey().ToBase58(), true)

	env.sendExternalAppChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    testutil.NewRandomAccount(t).ToProto(),
						Nonce:            make([]byte, 24),
						EncryptedPayload: []byte("secret"),
					},
				},
			},
		},
	}, testExternalAppDomain, true, owner)

	for _, isMuted := range []bool{true, true, false, false, true, false, true} {
		setMuteStateReq := &chatpb.SetMuteStateRequest{
			Owner:   owner.ToProto(),
			ChatId:  chatId.ToProto(),
			IsMuted: isMuted,
		}
		setMuteStateReq.Signature = signProtoMessage(t, setMuteStateReq, owner, false)

		setSubscripionStatusResp, err := env.client.SetMuteState(env.ctx, setMuteStateReq)
		require.NoError(t, err)
		assert.Equal(t, chatpb.SetMuteStateResponse_OK, setSubscripionStatusResp.Result)

		getChatsReq := &chatpb.GetChatsRequest{
			Owner: owner.ToProto(),
		}
		getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

		getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
		require.NoError(t, err)
		assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
		require.Len(t, getChatsResp.Chats, 1)
		assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
		assert.Equal(t, isMuted, getChatsResp.Chats[0].IsMuted)
	}
}

func TestChatMuteState_ChatNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId("test.com", owner.PublicKey().ToBase58(), false)

	setMuteStateReq := &chatpb.SetMuteStateRequest{
		Owner:   owner.ToProto(),
		ChatId:  chatId.ToProto(),
		IsMuted: false,
	}
	setMuteStateReq.Signature = signProtoMessage(t, setMuteStateReq, owner, false)

	setMuteStateResp, err := env.client.SetMuteState(env.ctx, setMuteStateReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.SetMuteStateResponse_CHAT_NOT_FOUND, setMuteStateResp.Result)
}

func TestChatMuteState_CantMute(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId(chat_util.TestCantMuteName, owner.PublicKey().ToBase58(), true)

	env.sendInternalChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: chatpb.ExchangeDataContent_GAVE,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: &transactionpb.ExchangeData{
								Currency:     "usd",
								ExchangeRate: 0.1,
								NativeAmount: 4.2,
								Quarks:       kin.ToQuarks(42),
							},
						},
					},
				},
			},
		},
	}, chat_util.TestCantMuteName, owner)

	setMuteStateReq := &chatpb.SetMuteStateRequest{
		Owner:   owner.ToProto(),
		ChatId:  chatId.ToProto(),
		IsMuted: true,
	}
	setMuteStateReq.Signature = signProtoMessage(t, setMuteStateReq, owner, false)

	setMuteStateResp, err := env.client.SetMuteState(env.ctx, setMuteStateReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.SetMuteStateResponse_CANT_MUTE, setMuteStateResp.Result)

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

	getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
	require.Len(t, getChatsResp.Chats, 1)
	assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
	assert.False(t, getChatsResp.Chats[0].IsMuted)
}

func TestChatSubscriptionState_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	testExternalAppDomain := "test.com"

	chatId := chat.GetChatId(testExternalAppDomain, owner.PublicKey().ToBase58(), true)

	env.sendExternalAppChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_NaclBox{
					NaclBox: &chatpb.NaclBoxEncryptedContent{
						PeerPublicKey:    testutil.NewRandomAccount(t).ToProto(),
						Nonce:            make([]byte, 24),
						EncryptedPayload: []byte("secret"),
					},
				},
			},
		},
	}, testExternalAppDomain, true, owner)

	for _, isSubscribed := range []bool{false, false, true, true, false, true, false} {
		setSubscriptionStateReq := &chatpb.SetSubscriptionStateRequest{
			Owner:        owner.ToProto(),
			ChatId:       chatId.ToProto(),
			IsSubscribed: isSubscribed,
		}
		setSubscriptionStateReq.Signature = signProtoMessage(t, setSubscriptionStateReq, owner, false)

		setSubscripionStatusResp, err := env.client.SetSubscriptionState(env.ctx, setSubscriptionStateReq)
		require.NoError(t, err)
		assert.Equal(t, chatpb.SetSubscriptionStateResponse_OK, setSubscripionStatusResp.Result)

		getChatsReq := &chatpb.GetChatsRequest{
			Owner: owner.ToProto(),
		}
		getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

		getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
		require.NoError(t, err)
		assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
		require.Len(t, getChatsResp.Chats, 1)
		assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
		assert.Equal(t, isSubscribed, getChatsResp.Chats[0].IsSubscribed)

		if isSubscribed {
			assert.EqualValues(t, 1, getChatsResp.Chats[0].NumUnread)
		} else {
			assert.EqualValues(t, 0, getChatsResp.Chats[0].NumUnread)
		}
	}
}

func TestChatSubscriptionState_ChatNotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId("test.com", owner.PublicKey().ToBase58(), false)

	setSubscriptionStateReq := &chatpb.SetSubscriptionStateRequest{
		Owner:        owner.ToProto(),
		ChatId:       chatId.ToProto(),
		IsSubscribed: false,
	}
	setSubscriptionStateReq.Signature = signProtoMessage(t, setSubscriptionStateReq, owner, false)

	setSubscripionStatusResp, err := env.client.SetSubscriptionState(env.ctx, setSubscriptionStateReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.SetSubscriptionStateResponse_CHAT_NOT_FOUND, setSubscripionStatusResp.Result)
}

func TestChatSubscriptionState_CantUnsubscribe(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	env.setupUserWithLocale(t, owner, language.English)

	chatId := chat.GetChatId(chat_util.TestCantUnsubscribeName, owner.PublicKey().ToBase58(), true)

	env.sendInternalChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: chatpb.ExchangeDataContent_GAVE,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: &transactionpb.ExchangeData{
								Currency:     "usd",
								ExchangeRate: 0.1,
								NativeAmount: 4.2,
								Quarks:       kin.ToQuarks(42),
							},
						},
					},
				},
			},
		},
	}, chat_util.TestCantUnsubscribeName, owner)

	setSubscriptionStateReq := &chatpb.SetSubscriptionStateRequest{
		Owner:        owner.ToProto(),
		ChatId:       chatId.ToProto(),
		IsSubscribed: false,
	}
	setSubscriptionStateReq.Signature = signProtoMessage(t, setSubscriptionStateReq, owner, false)

	setSubscripionStatusResp, err := env.client.SetSubscriptionState(env.ctx, setSubscriptionStateReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.SetSubscriptionStateResponse_CANT_UNSUBSCRIBE, setSubscripionStatusResp.Result)

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, false)

	getChatsResp, err := env.client.GetChats(env.ctx, getChatsReq)
	require.NoError(t, err)
	assert.Equal(t, chatpb.GetChatsResponse_OK, getChatsResp.Result)
	require.Len(t, getChatsResp.Chats, 1)
	assert.Equal(t, chatId[:], getChatsResp.Chats[0].ChatId.Value)
	assert.True(t, getChatsResp.Chats[0].IsSubscribed)
}

func TestUnauthorizedAccess(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	owner := testutil.NewRandomAccount(t)
	maliciousUser := testutil.NewRandomAccount(t)

	chatId := chat.GetChatId(chat_util.CodeTeamName, owner.PublicKey().ToBase58(), true)

	env.sendInternalChatMessage(t, &chatpb.ChatMessage{
		MessageId: &chatpb.ChatMessageId{
			Value: testutil.NewRandomAccount(t).ToProto().Value,
		},
		Ts: timestamppb.Now(),
		Content: []*chatpb.Content{
			{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						KeyOrText: "msg.body.key",
					},
				},
			},
		},
	}, chat_util.CodeTeamName, owner)

	//
	// GetChats
	//

	getChatsReq := &chatpb.GetChatsRequest{
		Owner: owner.ToProto(),
	}
	getChatsReq.Signature = signProtoMessage(t, getChatsReq, owner, true)

	_, err := env.client.GetChats(env.ctx, getChatsReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	//
	// GetMessages
	//

	getMessagesReq := &chatpb.GetMessagesRequest{
		ChatId: chatId.ToProto(),
		Owner:  owner.ToProto(),
	}
	getMessagesReq.Signature = signProtoMessage(t, getMessagesReq, owner, true)

	_, err = env.client.GetMessages(env.ctx, getMessagesReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	getMessagesReq = &chatpb.GetMessagesRequest{
		ChatId: chatId.ToProto(),
		Owner:  maliciousUser.ToProto(),
	}
	getMessagesReq.Signature = signProtoMessage(t, getMessagesReq, maliciousUser, false)

	_, err = env.client.GetMessages(env.ctx, getMessagesReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	//
	// AdvancePointer
	//

	advancePointerReq := &chatpb.AdvancePointerRequest{
		Owner:  owner.ToProto(),
		ChatId: chatId.ToProto(),
		Pointer: &chatpb.Pointer{
			Kind: chatpb.Pointer_READ,
			Value: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
		},
	}
	advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, maliciousUser, false)
	_, err = env.client.AdvancePointer(env.ctx, advancePointerReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	advancePointerReq = &chatpb.AdvancePointerRequest{
		Owner:  maliciousUser.ToProto(),
		ChatId: chatId.ToProto(),
		Pointer: &chatpb.Pointer{
			Kind: chatpb.Pointer_READ,
			Value: &chatpb.ChatMessageId{
				Value: testutil.NewRandomAccount(t).ToProto().Value,
			},
		},
	}
	advancePointerReq.Signature = signProtoMessage(t, advancePointerReq, maliciousUser, false)
	_, err = env.client.AdvancePointer(env.ctx, advancePointerReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	//
	// SetMuteState
	//

	setMuteStateReq := &chatpb.SetMuteStateRequest{
		Owner:   owner.ToProto(),
		ChatId:  chatId.ToProto(),
		IsMuted: true,
	}
	setMuteStateReq.Signature = signProtoMessage(t, setMuteStateReq, maliciousUser, false)
	_, err = env.client.SetMuteState(env.ctx, setMuteStateReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	setMuteStateReq = &chatpb.SetMuteStateRequest{
		Owner:   maliciousUser.ToProto(),
		ChatId:  chatId.ToProto(),
		IsMuted: true,
	}
	setMuteStateReq.Signature = signProtoMessage(t, setMuteStateReq, maliciousUser, false)
	_, err = env.client.SetMuteState(env.ctx, setMuteStateReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	//
	// SetSubscriptionState
	//

	setSubscriptionStateReq := &chatpb.SetSubscriptionStateRequest{
		Owner:        owner.ToProto(),
		ChatId:       chatId.ToProto(),
		IsSubscribed: false,
	}
	setSubscriptionStateReq.Signature = signProtoMessage(t, setSubscriptionStateReq, maliciousUser, false)
	_, err = env.client.SetSubscriptionState(env.ctx, setSubscriptionStateReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	setSubscriptionStateReq = &chatpb.SetSubscriptionStateRequest{
		Owner:        maliciousUser.ToProto(),
		ChatId:       chatId.ToProto(),
		IsSubscribed: false,
	}
	setSubscriptionStateReq.Signature = signProtoMessage(t, setSubscriptionStateReq, maliciousUser, false)
	_, err = env.client.SetSubscriptionState(env.ctx, setSubscriptionStateReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)
}

type testEnv struct {
	ctx    context.Context
	client chatpb.ChatClient
	server *server
	data   code_data.Provider
}

func setup(t *testing.T) (env *testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env = &testEnv{
		ctx:    context.Background(),
		client: chatpb.NewChatClient(conn),
		data:   code_data.NewTestDataProvider(),
	}

	s := NewChatServer(env.data, auth_util.NewRPCSignatureVerifier(env.data), memory_push.NewPushProvider())
	env.server = s.(*server)

	serv.RegisterService(func(server *grpc.Server) {
		chatpb.RegisterChatServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func (e *testEnv) sendExternalAppChatMessage(t *testing.T, msg *chatpb.ChatMessage, domain string, isVerified bool, recipient *common.Account) {
	_, err := chat_util.SendChatMessage(e.ctx, e.data, domain, chat.ChatTypeExternalApp, isVerified, recipient, msg, false)
	require.NoError(t, err)
}

func (e *testEnv) sendInternalChatMessage(t *testing.T, msg *chatpb.ChatMessage, chatTitle string, recipient *common.Account) {
	_, err := chat_util.SendChatMessage(e.ctx, e.data, chatTitle, chat.ChatTypeInternal, true, recipient, msg, false)
	require.NoError(t, err)
}

func (e *testEnv) setupUserWithLocale(t *testing.T, owner *common.Account, locale language.Tag) {
	phoneNumber := "+12223334444"
	containerId := user.NewDataContainerID()

	phoneVerificationRecord := &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   owner.PublicKey().ToBase58(),
		LastVerifiedAt: time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, e.data.SavePhoneVerification(e.ctx, phoneVerificationRecord))

	containerRecord := &storage.Record{
		ID:           containerId,
		OwnerAccount: owner.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, e.data.PutUserDataContainer(e.ctx, containerRecord))

	userPreferencesRecord := preferences.GetDefaultPreferences(containerId)
	userPreferencesRecord.Locale = locale
	require.NoError(t, e.data.SaveUserPreferences(e.ctx, userPreferencesRecord))
}

func signProtoMessage(t *testing.T, msg proto.Message, signer *common.Account, simulateInvalidSignature bool) *commonpb.Signature {
	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err)

	if simulateInvalidSignature {
		signer = testutil.NewRandomAccount(t)
	}

	signature, err := signer.Sign(msgBytes)
	require.NoError(t, err)

	return &commonpb.Signature{
		Value: signature,
	}
}
