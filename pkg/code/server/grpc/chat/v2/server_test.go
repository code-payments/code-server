package chat_v2

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/currency"
	pushmemory "github.com/code-payments/code-server/pkg/push/memory"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func TestServerHappy(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	userA := testutil.NewRandomAccount(t)
	userB := testutil.NewRandomAccount(t)

	ctx := context.Background()
	for i, u := range []*common.Account{userA, userB} {
		tipAddr, err := u.ToMessagingAccount(common.KinMintAccount)
		require.NoError(t, err)

		userSuffix := string(rune('a' + i))

		err = env.data.SaveTwitterUser(ctx, &twitter.Record{
			Username:      fmt.Sprintf("username-%s", userSuffix),
			Name:          fmt.Sprintf("name-%s", userSuffix),
			ProfilePicUrl: fmt.Sprintf("pp-%s", userSuffix),
			TipAddress:    tipAddr.PublicKey().ToBase58(),
			LastUpdatedAt: time.Now(),
			CreatedAt:     time.Now(),
		})
		require.NoError(t, err)

		err = env.data.CreateAccountInfo(ctx, &account.Record{
			OwnerAccount:     u.String(),
			AuthorityAccount: u.String(),
			TokenAccount:     base58.Encode(u.MustToChatMemberId()),
			MintAccount:      common.KinMintAccount.String(),
			AccountType:      commonpb.AccountType_PRIMARY,
			CreatedAt:        time.Now(),
		})
		require.NoError(t, err)
	}

	chatId := chat.GetTwoWayChatId(userA.MustToChatMemberId(), userB.MustToChatMemberId())
	intentId := bytes.Repeat([]byte{1}, 32)
	err := env.data.SaveIntent(ctx, &intent.Record{
		IntentId:              base58.Encode(intentId),
		IntentType:            intent.SendPrivatePayment,
		InitiatorOwnerAccount: userA.String(),
		SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
			DestinationTokenAccount: userB.String(),
			Quantity:                10,
			ExchangeCurrency:        currency.USD,
			ExchangeRate:            10,
			UsdMarketValue:          10.0,
			NativeAmount:            1,
			IsChat:                  true,
			ChatId:                  base58.Encode(chatId[:]),
		},
		State:     intent.StateConfirmed,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	t.Run("Initial State", func(t *testing.T) {
		req := &chatpb.GetChatsRequest{Owner: userA.ToProto()}
		req.Signature = signProtoMessage(t, req, userA, false)

		chats, err := env.client.GetChats(ctx, req)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatsResponse_OK, chats.Result)
		require.Empty(t, chats.Chats)
	})

	t.Run("StartChat", func(t *testing.T) {
		req := &chatpb.StartChatRequest{
			Owner: userA.ToProto(),
			Parameters: &chatpb.StartChatRequest_TwoWayChat{
				TwoWayChat: &chatpb.StartTwoWayChatParameters{
					OtherUser: &commonpb.SolanaAccountId{Value: userB.MustToChatMemberId()},
					IntentId:  &commonpb.IntentId{Value: intentId},
				},
			},
		}
		req.Signature = signProtoMessage(t, req, userA, false)

		resp, err := env.client.StartChat(ctx, req)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, resp.Result)
		require.NotEmpty(t, resp.GetChat().GetChatId())

		expectedMeta := &chatpb.Metadata{
			ChatId: resp.Chat.ChatId,
			Type:   chatpb.ChatType_TWO_WAY,
			Cursor: &chatpb.Cursor{Value: resp.Chat.ChatId.Value},
			Title:  "",
			Members: []*chatpb.Member{
				{
					MemberId: userA.MustToChatMemberId().ToProto(),
					Identity: &chatpb.MemberIdentity{
						Platform:      chatpb.Platform_TWITTER,
						Username:      "username-a",
						DisplayName:   "name-a",
						ProfilePicUrl: "pp-a",
					},
					IsSelf: true,
				},
				{
					MemberId: userB.MustToChatMemberId().ToProto(),
					Identity: &chatpb.MemberIdentity{
						Platform:      chatpb.Platform_TWITTER,
						Username:      "username-b",
						DisplayName:   "name-b",
						ProfilePicUrl: "pp-b",
					},
				},
			},
		}

		slices.SortFunc(expectedMeta.Members, func(a, b *chatpb.Member) int {
			return bytes.Compare(a.MemberId.Value, b.MemberId.Value)
		})
		slices.SortFunc(resp.Chat.Members, func(a, b *chatpb.Member) int {
			return bytes.Compare(a.MemberId.Value, b.MemberId.Value)
		})

		require.NoError(t, testutil.ProtoEqual(expectedMeta, resp.Chat))

		for _, u := range []*common.Account{userA, userB} {
			getChats := &chatpb.GetChatsRequest{Owner: u.ToProto()}
			getChats.Signature = signProtoMessage(t, getChats, u, false)

			for _, member := range resp.Chat.Members {
				member.IsSelf = bytes.Equal(u.MustToChatMemberId(), member.MemberId.Value)
			}

			chats, err := env.client.GetChats(ctx, getChats)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatsResponse_OK, chats.Result)
			require.Len(t, chats.Chats, 1)

			slices.SortFunc(chats.Chats[0].Members, func(a, b *chatpb.Member) int {
				return bytes.Compare(a.MemberId.Value, b.MemberId.Value)
			})

			require.NoError(t, testutil.ProtoEqual(resp.Chat, chats.Chats[0]))
		}
	})

	var messages []*chatpb.Message
	t.Run("Send Messages", func(t *testing.T) {
		for _, u := range []*common.Account{userA, userB} {
			for i := 0; i < 5; i++ {
				req := &chatpb.SendMessageRequest{
					ChatId: chatId.ToProto(),
					Owner:  u.ToProto(),
					Content: []*chatpb.Content{
						{
							Type: &chatpb.Content_Text{
								Text: &chatpb.TextContent{
									Text: fmt.Sprintf("message-%d", i),
								},
							},
						},
					},
				}
				req.Signature = signProtoMessage(t, req, u, false)

				resp, err := env.client.SendMessage(ctx, req)
				require.NoError(t, err)
				require.Equal(t, chatpb.SendMessageResponse_OK, resp.Result)
				messages = append(messages, resp.GetMessage())

				// TODO: Hack on message generation...again.
				time.Sleep(time.Millisecond)
			}
		}

		for _, u := range []*common.Account{userA, userB} {
			req := &chatpb.GetChatsRequest{Owner: u.ToProto()}
			req.Signature = signProtoMessage(t, req, u, false)

			resp, err := env.client.GetChats(ctx, req)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatsResponse_OK, resp.Result)

			// 5 unread _each_
			require.EqualValues(t, 5, resp.Chats[0].NumUnread)
		}
	})

	t.Run("Get Messages", func(t *testing.T) {
		for _, u := range []*common.Account{userA, userB} {
			req := &chatpb.GetMessagesRequest{
				ChatId: chatId.ToProto(),
				Owner:  u.ToProto(),
			}
			req.Signature = signProtoMessage(t, req, u, false)

			resp, err := env.client.GetMessages(ctx, req)
			require.NoError(t, err)
			require.NoError(t, testutil.ProtoSliceEqual(messages, resp.GetMessages()))

			req.Cursor = resp.Messages[1].GetCursor()
			req.Signature = nil
			req.Signature = signProtoMessage(t, req, u, false)

			resp, err = env.client.GetMessages(ctx, req)
			require.NoError(t, err)
			require.NoError(t, testutil.ProtoSliceEqual(messages[2:], resp.GetMessages()))
		}
	})

	t.Run("Advance Pointer", func(t *testing.T) {
		for _, tc := range []struct {
			offset int
			user   *common.Account
		}{
			{offset: 5 + 2, user: userA},
			{offset: 0 + 2, user: userB},
		} {
			req := &chatpb.AdvancePointerRequest{
				ChatId: chatId.ToProto(),
				Pointer: &chatpb.Pointer{
					Type:     chatpb.PointerType_READ,
					Value:    messages[tc.offset].MessageId,
					MemberId: tc.user.MustToChatMemberId().ToProto(),
				},
				Owner: tc.user.ToProto(),
			}
			req.Signature = signProtoMessage(t, req, tc.user, false)

			resp, err := env.client.AdvancePointer(ctx, req)
			require.NoError(t, err)
			require.Equal(t, chatpb.AdvancePointerResponse_OK, resp.Result)

			getChats := &chatpb.GetChatsRequest{Owner: tc.user.ToProto()}
			getChats.Signature = signProtoMessage(t, getChats, tc.user, false)

			chats, err := env.client.GetChats(ctx, getChats)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatsResponse_OK, chats.Result)
			require.EqualValues(t, 2, chats.Chats[0].NumUnread)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		client, err := env.client.StreamChatEvents(ctx)
		require.NoError(t, err)

		req := &chatpb.OpenChatEventStream{
			ChatId:    chatId.ToProto(),
			Owner:     userA.ToProto(),
			Signature: nil,
		}
		req.Signature = signProtoMessage(t, req, userA, false)

		err = client.Send(&chatpb.StreamChatEventsRequest{
			Type: &chatpb.StreamChatEventsRequest_OpenStream{
				OpenStream: req,
			},
		})
		require.NoError(t, err)

		// expect some amount of flushes
		var streamedMessages []*chatpb.Message
		for {
			resp, err := client.Recv()
			require.NoError(t, err)

			switch typed := resp.Type.(type) {
			case *chatpb.StreamChatEventsResponse_Error:
				require.FailNow(t, typed.Error.String())
			case *chatpb.StreamChatEventsResponse_Ping:
				_ = client.Send(&chatpb.StreamChatEventsRequest{
					Type: &chatpb.StreamChatEventsRequest_Pong{
						Pong: &commonpb.ClientPong{Timestamp: timestamppb.Now()},
					},
				})

			case *chatpb.StreamChatEventsResponse_Events:
				for _, e := range typed.Events.Events {
					if m := e.GetMessage(); m != nil {
						streamedMessages = append(streamedMessages, m)
						if len(streamedMessages) == len(messages) {
							break
						}
					}
				}

			default:
			}

			if len(streamedMessages) == len(messages) {
				break
			}
		}

		require.True(t, slices.IsSortedFunc(streamedMessages, func(a, b *chatpb.Message) int {
			return -1 * bytes.Compare(a.MessageId.Value, b.MessageId.Value)
		}))
		slices.Reverse(streamedMessages)
		require.NoError(t, testutil.ProtoSliceEqual(messages, streamedMessages))
	})
}

type testEnv struct {
	ctx    context.Context
	client chatpb.ChatClient
	server *Server
	data   data.Provider
}

func setup(t *testing.T) (env *testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env = &testEnv{
		ctx:    context.Background(),
		client: chatpb.NewChatClient(conn),
		data:   data.NewTestDataProvider(),
	}

	env.server = NewChatServer(
		env.data,
		auth_util.NewRPCSignatureVerifier(env.data),
		pushmemory.NewPushProvider(),
	)

	serv.RegisterService(func(server *grpc.Server) {
		chatpb.RegisterChatServer(server, env.server)
	})

	testutil.SetupRandomSubsidizer(t, env.data)

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
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
