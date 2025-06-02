package messaging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/testutil"
)

func TestRendezvousProcess_HappyPath_OpenBeforeSend(t *testing.T) {
	for _, enableMultiServer := range []bool{true, false} {
		for _, enableKeepAlive := range []bool{true, false} {
			func() {
				env, cleanup := setup(t, enableMultiServer)
				defer cleanup()

				rendezvousKey := testutil.NewRandomAccount(t)

				env.client1.openMessageStream(t, rendezvousKey, enableKeepAlive)
				env.server1.assertInitialRendezvousRecordSaved(t, rendezvousKey)
				time.Sleep(500 * time.Millisecond) // allow async flush to finish

				sendMessageCall := env.client2.sendRequestToGrabBillMessage(t, rendezvousKey)
				sendMessageCall.requireSuccess(t)

				records := env.server1.getMessages(t, rendezvousKey)
				require.Len(t, records, 1)

				messages := env.client1.receiveMessagesInRealTime(t, rendezvousKey)
				require.Len(t, messages, 1)

				env.client1.closeMessageStream(t, rendezvousKey)
				env.server1.assertRendezvousRecordDeleted(t, rendezvousKey)

				message := messages[0]
				assert.Equal(t, sendMessageCall.resp.MessageId.Value, message.Id.Value)

				env.client1.ackMessages(t, rendezvousKey, message.Id)
				env.server1.assertNoMessages(t, rendezvousKey)
			}()
		}
	}
}

func TestRendezvousProcess_HappyPath_OpenAfterSend(t *testing.T) {
	for _, enableMultiServer := range []bool{true, false} {
		for _, enableKeepAlive := range []bool{true, false} {
			func() {
				env, cleanup := setup(t, enableMultiServer)
				defer cleanup()

				rendezvousKey := testutil.NewRandomAccount(t)
				sendMessageCall := env.client2.sendRequestToGrabBillMessage(t, rendezvousKey)
				sendMessageCall.requireSuccess(t)

				records := env.server1.getMessages(t, rendezvousKey)
				require.Len(t, records, 1)

				env.client1.openMessageStream(t, rendezvousKey, enableKeepAlive)
				env.server1.assertInitialRendezvousRecordSaved(t, rendezvousKey)

				messages := env.client1.receiveMessagesInRealTime(t, rendezvousKey)
				require.Len(t, messages, 1)

				env.client1.closeMessageStream(t, rendezvousKey)
				env.server1.assertRendezvousRecordDeleted(t, rendezvousKey)

				message := messages[0]
				assert.Equal(t, sendMessageCall.resp.MessageId.Value, message.Id.Value)

				env.client1.ackMessages(t, rendezvousKey, message.Id)
				env.server1.assertNoMessages(t, rendezvousKey)
			}()
		}
	}
}

func TestRendezvousProcess_MultipleOpenStreams(t *testing.T) {
	for i := 0; i < 32; i++ {
		for _, enableMultiServer := range []bool{true, false} {
			func() {
				env, cleanup := setup(t, enableMultiServer)
				defer cleanup()

				rendezvousKey := testutil.NewRandomAccount(t)

				for j := 0; j < 10; j++ {
					env.client1.openMessageStream(t, rendezvousKey, j%2 == 0)
					env.client2.openMessageStream(t, rendezvousKey, j%2 == 1)
				}
				time.Sleep(500 * time.Millisecond) // allow async flush to finish

				senderClient := env.client2
				if i%3 == 0 {
					senderClient = env.client1
				}

				sendMessageCall := senderClient.sendRequestToGrabBillMessage(t, rendezvousKey)
				sendMessageCall.requireSuccess(t)

				records := env.server1.getMessages(t, rendezvousKey)
				require.Len(t, records, 1)

				messages := env.client1.receiveMessagesInRealTime(t, rendezvousKey)
				messages = append(messages, env.client2.receiveMessagesInRealTime(t, rendezvousKey)...)
				require.Len(t, messages, 1)

				env.client1.closeMessageStream(t, rendezvousKey)

				message := messages[0]
				assert.Equal(t, sendMessageCall.resp.MessageId.Value, message.Id.Value)

				env.client1.ackMessages(t, rendezvousKey, message.Id)
				env.server1.assertNoMessages(t, rendezvousKey)
			}()
		}
	}
}

func TestRendezvousProcess_InternallyGeneratedMessage(t *testing.T) {
	for _, enableMultiServer := range []bool{true, false} {
		func() {
			env, cleanup := setup(t, enableMultiServer)
			defer cleanup()

			rendezvousKey := testutil.NewRandomAccount(t)

			env.client1.openMessageStream(t, rendezvousKey, false)
			time.Sleep(500 * time.Millisecond) // allow async flush to finish

			expectedMessage := &messagingpb.Message{
				Kind: &messagingpb.Message_CodeScanned{
					CodeScanned: &messagingpb.CodeScanned{
						Timestamp: timestamppb.Now(),
					},
				},
			}
			serverEnv := env.server1
			if enableMultiServer {
				serverEnv = env.server2
			}
			messageId, err := serverEnv.server.InternallyCreateMessage(serverEnv.ctx, rendezvousKey, expectedMessage)
			require.NoError(t, err)

			records := env.server1.getMessages(t, rendezvousKey)
			require.Len(t, records, 1)
			assert.Equal(t, rendezvousKey.PublicKey().ToBase58(), records[0].Account)
			assert.Equal(t, messageId[:], records[0].MessageID[:])

			var savedProtoMessage messagingpb.Message
			require.NoError(t, proto.Unmarshal(records[0].Message, &savedProtoMessage))

			assert.Equal(t, messageId[:], savedProtoMessage.Id.Value)
			require.NotNil(t, savedProtoMessage.GetCodeScanned())
			assert.True(t, proto.Equal(expectedMessage.GetCodeScanned(), savedProtoMessage.GetCodeScanned()))
			assert.Nil(t, savedProtoMessage.SendMessageRequestSignature)

			messages := env.client1.receiveMessagesInRealTime(t, rendezvousKey)
			require.Len(t, messages, 1)

			env.client1.closeMessageStream(t, rendezvousKey)

			actualMessage := messages[0]
			assert.Equal(t, messageId[:], actualMessage.Id.Value)
			assert.True(t, proto.Equal(expectedMessage, actualMessage))

			env.client1.ackMessages(t, rendezvousKey, actualMessage.Id)
			env.server1.assertNoMessages(t, rendezvousKey)
		}()
	}
}

func TestSendMessage_RequestToGrabBill_HappyPath(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	rendezvousKey := testutil.NewRandomAccount(t)
	sendMessageCall := env.client2.sendRequestToGrabBillMessage(t, rendezvousKey)
	sendMessageCall.requireSuccess(t)

	records := env.server1.getMessages(t, rendezvousKey)
	require.Len(t, records, 1)
	assert.Equal(t, rendezvousKey.PublicKey().ToBase58(), records[0].Account)
	assert.Equal(t, sendMessageCall.resp.MessageId.Value, records[0].MessageID[:])

	var savedProtoMessage messagingpb.Message
	require.NoError(t, proto.Unmarshal(records[0].Message, &savedProtoMessage))

	assert.Equal(t, sendMessageCall.resp.MessageId.Value, savedProtoMessage.Id.Value)
	require.NotNil(t, savedProtoMessage.GetRequestToGrabBill())
	assert.Equal(t, sendMessageCall.req.Message.GetRequestToGrabBill().RequestorAccount.Value, savedProtoMessage.GetRequestToGrabBill().RequestorAccount.Value)
	assert.Equal(t, sendMessageCall.req.Signature.Value, savedProtoMessage.SendMessageRequestSignature.Value)

	env.client1.openMessageStream(t, rendezvousKey, false)
	messages := env.client1.receiveMessagesInRealTime(t, rendezvousKey)
	env.client1.closeMessageStream(t, rendezvousKey)
	require.Len(t, messages, 1)
	assert.True(t, proto.Equal(&savedProtoMessage, messages[0]))
}

func TestSendMessage_RequestToGrabBill_Validation(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	rendezvousKey := testutil.NewRandomAccount(t)

	env.client1.resetConf()
	env.client1.conf.simulateAccountNotCodeAccount = true
	sendMessageCall := env.client1.sendRequestToGrabBillMessage(t, rendezvousKey)
	sendMessageCall.assertInvalidMessageError(t, "requestor account must be a primary account")
	env.server1.assertNoMessages(t, rendezvousKey)

	env.client1.resetConf()
	env.client1.conf.simulateInvalidAccountType = true
	sendMessageCall = env.client1.sendRequestToGrabBillMessage(t, rendezvousKey)
	sendMessageCall.assertInvalidMessageError(t, "requestor account must be a primary account")
	env.server1.assertNoMessages(t, rendezvousKey)
}

func TestSendMessage_InvalidRendezvousKeySignature(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	rendezvousKey := testutil.NewRandomAccount(t)

	env.client1.conf.simulateInvalidRequestSignature = true
	sendMessageCall := env.client1.sendRequestToGrabBillMessage(t, testutil.NewRandomAccount(t))
	sendMessageCall.assertUnauthenticatedError(t, "")
	env.server1.assertNoMessages(t, rendezvousKey)
}

func TestMessagePolling_HappyPath(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	rendezvousKey := testutil.NewRandomAccount(t)
	sendMessageCall := env.client2.sendRequestToGrabBillMessage(t, rendezvousKey)
	sendMessageCall.requireSuccess(t)

	messages := env.client1.pollForMessages(t, rendezvousKey)
	require.Len(t, messages, 1)

	message := messages[0]
	assert.Equal(t, sendMessageCall.resp.MessageId.Value, message.Id.Value)
	assert.Equal(t, sendMessageCall.req.Signature.Value, message.SendMessageRequestSignature.Value)
	require.NotNil(t, message.GetRequestToGrabBill())
	assert.EqualValues(t, sendMessageCall.req.Message.GetRequestToGrabBill().RequestorAccount.Value, message.GetRequestToGrabBill().RequestorAccount.Value)

	env.client1.ackMessages(t, rendezvousKey, sendMessageCall.resp.MessageId)
	messages = env.client1.pollForMessages(t, rendezvousKey)
	require.Empty(t, messages)
}

func TestKeepAlive_HappyPath(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	absoluteTimeout := messageStreamWithoutKeepAliveTimeout

	start := time.Now()
	rendezvousKey := testutil.NewRandomAccount(t)
	env.client1.openMessageStream(t, rendezvousKey, true)
	env.server1.assertInitialRendezvousRecordSaved(t, rendezvousKey)

	pingCount := env.client1.waitUntilStreamTerminationOrTimeout(t, rendezvousKey, true, absoluteTimeout)
	assert.True(t, time.Since(start) >= absoluteTimeout)
	assert.True(t, pingCount >= int(absoluteTimeout/messageStreamPingDelay))
	assert.True(t, pingCount <= int(absoluteTimeout/messageStreamPingDelay)+2)
	env.server1.assertRendezvousRecordRefreshed(t, rendezvousKey)
}

func TestKeepAlive_UnresponsiveClient(t *testing.T) {
	env, cleanup := setup(t, false)
	defer cleanup()

	absoluteTimeout := messageStreamWithoutKeepAliveTimeout

	start := time.Now()
	rendezvousKey := testutil.NewRandomAccount(t)
	env.client1.openMessageStream(t, rendezvousKey, true)

	pingCount := env.client1.waitUntilStreamTerminationOrTimeout(t, rendezvousKey, false, absoluteTimeout)
	assert.True(t, time.Since(start) >= messageStreamKeepAliveRecvTimeout)
	assert.True(t, time.Since(start) <= messageStreamKeepAliveRecvTimeout+50*time.Millisecond)
	assert.True(t, pingCount >= int(messageStreamKeepAliveRecvTimeout/messageStreamPingDelay))
	assert.True(t, pingCount <= int(messageStreamKeepAliveRecvTimeout/messageStreamPingDelay)+1)
}
