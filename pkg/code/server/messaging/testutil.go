package messaging

import (
	"context"
	"crypto/ed25519"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/messaging"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	client1 *clientEnv
	client2 *clientEnv
	server1 *serverEnv
	server2 *serverEnv
}

func setup(t *testing.T, enableMultiServer bool) (env testEnv, cleanup func()) {
	conn1, serv1, err := testutil.NewServer()
	require.NoError(t, err)

	conn2, serv2, err := testutil.NewServer()
	require.NoError(t, err)

	data := code_data.NewTestDataProvider()

	env.client1 = &clientEnv{
		ctx:              context.Background(),
		client:           messagingpb.NewMessagingClient(conn1),
		conf:             &clientConf{},
		streams:          make(map[string][]*cancellableStream),
		directDataAccess: data,
	}
	env.client2 = &clientEnv{
		ctx:              context.Background(),
		client:           messagingpb.NewMessagingClient(conn1),
		conf:             &clientConf{},
		streams:          make(map[string][]*cancellableStream),
		directDataAccess: data,
	}
	if enableMultiServer {
		env.client2.client = messagingpb.NewMessagingClient(conn2)
	}

	subsidizer := testutil.SetupRandomSubsidizer(t, data)

	require.NoError(t, data.ImportExchangeRates(context.Background(), &currency.MultiRateRecord{
		Time: exchange_rate_util.GetLatestExchangeRateTime(),
		Rates: map[string]float64{
			"usd": 0.1,
		},
	}))

	s1 := NewMessagingClientAndServer(data, auth.NewRPCSignatureVerifier(data), conn1.Target(), withManualTestOverrides(&testOverrides{}))
	env.server1 = &serverEnv{
		ctx:        context.Background(),
		server:     s1,
		subsidizer: subsidizer,
	}

	s2 := NewMessagingClientAndServer(data, auth.NewRPCSignatureVerifier(data), conn2.Target(), withManualTestOverrides(&testOverrides{}))
	env.server2 = &serverEnv{
		ctx:        context.Background(),
		server:     s2,
		subsidizer: subsidizer,
	}

	serv1.RegisterService(func(server *grpc.Server) {
		messagingpb.RegisterMessagingServer(server, s1)
	})
	serv2.RegisterService(func(server *grpc.Server) {
		messagingpb.RegisterMessagingServer(server, s2)
	})

	cleanup1, err := serv1.Serve()
	require.NoError(t, err)

	cleanup2, err := serv2.Serve()
	require.NoError(t, err)

	return env, func() {
		cleanup1()
		cleanup2()
	}
}

type serverEnv struct {
	ctx        context.Context
	server     *server
	subsidizer *common.Account
}

func (s *serverEnv) getMessages(t *testing.T, rendezvousKey *common.Account) []*messaging.Record {
	messages, err := s.server.data.GetMessages(s.ctx, rendezvousKey.PublicKey().ToBase58())
	require.NoError(t, err)
	return messages
}

func (s *serverEnv) assertNoMessages(t *testing.T, rendezvousKey *common.Account) {
	messages, err := s.server.data.GetMessages(s.ctx, rendezvousKey.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func (s *serverEnv) assertInitialRendezvousRecordSaved(t *testing.T, rendezvousKey *common.Account) {
	start := time.Now()

	for i := 0; i < 5; i++ {
		rendezvousRecord, err := s.server.data.GetRendezvous(s.ctx, rendezvousKey.PublicKey().ToBase58())
		if err == rendezvous.ErrNotFound {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		require.NoError(t, err)

		assert.Equal(t, rendezvousKey.PublicKey().ToBase58(), rendezvousRecord.Key)
		assert.Equal(t, s.server.broadcastAddress, rendezvousRecord.Address) // Note: assertion must be called on the expected server
		assert.True(t, start.Sub(rendezvousRecord.CreatedAt) <= 50*time.Millisecond)
		assert.True(t, start.Sub(rendezvousRecord.CreatedAt) >= -50*time.Millisecond)
		assert.Equal(t, rendezvousRecord.CreatedAt.Add(rendezvousRecordExpiryTime).Unix(), rendezvousRecord.ExpiresAt.Unix())
		return
	}

	require.Fail(t, "rendezvous record not saved")
}

func (s *serverEnv) assertRendezvousRecordRefreshed(t *testing.T, rendezvousKey *common.Account) {
	rendezvousRecord, err := s.server.data.GetRendezvous(s.ctx, rendezvousKey.PublicKey().ToBase58())
	require.NoError(t, err)
	assert.True(t, rendezvousRecord.ExpiresAt.After(time.Now()))
	assert.True(t, rendezvousRecord.ExpiresAt.Before(time.Now().Add(rendezvousRecordExpiryTime)))
}

func (s *serverEnv) assertRendezvousRecordDeleted(t *testing.T, rendezvousKey *common.Account) {
	for i := 0; i < 5; i++ {
		_, err := s.server.data.GetRendezvous(s.ctx, rendezvousKey.PublicKey().ToBase58())
		if err == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		assert.Equal(t, rendezvous.ErrNotFound, err)
	}
}

type cancellableStream struct {
	stream               messagingpb.Messaging_OpenMessageStreamClient
	streamWithKeepAlives messagingpb.Messaging_OpenMessageStreamWithKeepAliveClient
	cancel               func()
}

type clientConf struct {
	// Simulations for invalid account

	simulateInvalidAccountType    bool
	simulateAccountNotCodeAccount bool

	// Simulations for invalid exchange data

	simulateInvalidCurrency        bool
	simulateInvalidExchangeRate    bool
	simulateInvalidNativeAmount    bool
	simulateSmallNativeAmount      bool
	simulateLargeNativeAmount      bool
	simulateFractionalNativeAmount bool
	simulateFractionalQuarkAmount  bool

	// Simulations for invalid relationships

	simulateInvalidRelationship bool
	simulateInvalidDomain       bool
	simulateDoesntOwnDomain     bool

	// Simulations for invalid signatures

	simulateInvalidRequestSignature bool
	simulateInvalidMessageSignature bool

	// Simulations for invalid rendezvous keys

	simulateInvalidRendezvousKey bool

	// Simulations for invalid fee structures
	simulateLargeFeePercentage           bool
	simulateInvalidFeeCodeAccount        bool
	simulateInvalidFeeRelationship       bool
	simulateDuplicatedFeeTaker           bool
	simulateFeeTakerIsPaymentDestination bool
}

type clientEnv struct {
	ctx     context.Context
	client  messagingpb.MessagingClient
	conf    *clientConf
	streams map[string][]*cancellableStream

	// Direct data access to help test/pass validation checks
	directDataAccess code_data.Provider
}

func (c *clientEnv) openMessageStream(t *testing.T, rendezvousKey *common.Account, enableKeepAlive bool) {
	cancellableCtx, cancel := context.WithCancel(c.ctx)

	req := &messagingpb.OpenMessageStreamRequest{
		RendezvousKey: &messagingpb.RendezvousKey{
			Value: rendezvousKey.PublicKey().ToBytes(),
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(rendezvousKey.PrivateKey().ToBytes(), reqBytes),
	}

	if enableKeepAlive {
		streamer, err := c.client.OpenMessageStreamWithKeepAlive(cancellableCtx)
		require.NoError(t, err)

		require.NoError(t, streamer.Send(&messagingpb.OpenMessageStreamWithKeepAliveRequest{
			RequestOrPong: &messagingpb.OpenMessageStreamWithKeepAliveRequest_Request{
				Request: req,
			},
		}))

		c.streams[rendezvousKey.PublicKey().ToBase58()] = append(c.streams[rendezvousKey.PublicKey().ToBase58()], &cancellableStream{
			streamWithKeepAlives: streamer,
			cancel:               cancel,
		})
	} else {
		streamer, err := c.client.OpenMessageStream(cancellableCtx, req)
		require.NoError(t, err)
		c.streams[rendezvousKey.PublicKey().ToBase58()] = append(c.streams[rendezvousKey.PublicKey().ToBase58()], &cancellableStream{
			stream: streamer,
			cancel: cancel,
		})
	}
}

func (c *clientEnv) closeMessageStream(t *testing.T, rendezvousKey *common.Account) {
	streamers, ok := c.streams[rendezvousKey.PublicKey().ToBase58()]
	require.True(t, ok)
	for _, streamer := range streamers {
		streamer.cancel()
	}
	delete(c.streams, rendezvousKey.PublicKey().ToBase58())
}

func (c *clientEnv) receiveMessagesInRealTime(t *testing.T, rendezvousKey *common.Account) []*messagingpb.Message {
	streamers, ok := c.streams[rendezvousKey.PublicKey().ToBase58()]
	require.True(t, ok)

	for _, streamer := range streamers {
		if streamer.streamWithKeepAlives != nil {
			for {
				resp, err := streamer.streamWithKeepAlives.Recv()

				status, ok := status.FromError(err)
				if ok && status.Code() == codes.Aborted {
					// Try the next open stream
					break
				}

				require.NoError(t, err)

				switch typed := resp.ResponseOrPing.(type) {
				case *messagingpb.OpenMessageStreamWithKeepAliveResponse_Response:
					return typed.Response.Messages
				case *messagingpb.OpenMessageStreamWithKeepAliveResponse_Ping:
					err = streamer.streamWithKeepAlives.Send(&messagingpb.OpenMessageStreamWithKeepAliveRequest{
						RequestOrPong: &messagingpb.OpenMessageStreamWithKeepAliveRequest_Pong{
							Pong: &commonpb.ClientPong{
								Timestamp: timestamppb.Now(),
							},
						},
					})
					// Stream has been terminated
					if err != io.EOF {
						require.NoError(t, err)
					}
				default:
					require.Fail(t, "response and ping wasn't set")
				}
			}
		} else {
			resp, err := streamer.stream.Recv()

			status, ok := status.FromError(err)
			if ok && status.Code() == codes.Aborted {
				// Try the next open stream
				continue
			}

			require.NoError(t, err)
			return resp.Messages
		}
	}

	return nil
}

func (c *clientEnv) pollForMessages(t *testing.T, rendezvousKey *common.Account) []*messagingpb.Message {
	req := &messagingpb.PollMessagesRequest{
		RendezvousKey: &messagingpb.RendezvousKey{
			Value: rendezvousKey.PublicKey().ToBytes(),
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(rendezvousKey.PrivateKey().ToBytes(), reqBytes),
	}

	resp, err := c.client.PollMessages(c.ctx, req)
	require.NoError(t, err)
	return resp.Messages
}

func (c *clientEnv) waitUntilStreamTerminationOrTimeout(t *testing.T, rendezvousKey *common.Account, keepStreamAlive bool, timeout time.Duration) int {
	streamers, ok := c.streams[rendezvousKey.PublicKey().ToBase58()]
	require.True(t, ok)
	require.Len(t, streamers, 1)

	streamer := streamers[0]
	require.NotNil(t, streamer.streamWithKeepAlives)

	var pingCount int
	var lastPingTs time.Time
	start := time.Now()
	for {
		resp, err := streamer.streamWithKeepAlives.Recv()

		status, ok := status.FromError(err)
		if ok && status.Code() == codes.Aborted {
			return pingCount
		}

		require.NoError(t, err)

		switch typed := resp.ResponseOrPing.(type) {
		case *messagingpb.OpenMessageStreamWithKeepAliveResponse_Ping:
			pingTimeEsimate := start
			if pingCount > 0 {
				pingTimeEsimate = lastPingTs.Add(messageStreamPingDelay)
			}

			deltaPingTime := typed.Ping.Timestamp.AsTime().Sub(pingTimeEsimate)
			assert.True(t, deltaPingTime <= 50*time.Millisecond)
			assert.True(t, deltaPingTime >= -50*time.Millisecond)

			assert.Equal(t, messageStreamPingDelay, typed.Ping.PingDelay.AsDuration())
			if pingCount == 0 {
				// First ping should come immediately
				assert.True(t, time.Since(start) <= 50*time.Millisecond)
			} else {
				// Every other ping comes at the defined time interval
				assert.True(t, time.Since(lastPingTs) >= messageStreamPingDelay-50*time.Millisecond)
				assert.True(t, time.Since(lastPingTs) <= messageStreamPingDelay+50*time.Millisecond)
			}

			pingCount += 1
			lastPingTs = time.Now()

			if keepStreamAlive {
				require.NoError(t, streamer.streamWithKeepAlives.Send(&messagingpb.OpenMessageStreamWithKeepAliveRequest{
					RequestOrPong: &messagingpb.OpenMessageStreamWithKeepAliveRequest_Pong{
						Pong: &commonpb.ClientPong{
							Timestamp: timestamppb.Now(),
						},
					},
				}))
			}

			if time.Since(start) > timeout {
				return pingCount
			}
		case *messagingpb.OpenMessageStreamWithKeepAliveResponse_Response:
		default:
			require.Fail(t, "response and ping wasn't set")
		}
	}
}

type sendMessageCallMetadata struct {
	req  *messagingpb.SendMessageRequest
	resp *messagingpb.SendMessageResponse
	err  error
}

func (c *sendMessageCallMetadata) requireSuccess(t *testing.T) {
	require.NoError(t, c.err)
	require.Equal(t, messagingpb.SendMessageResponse_OK, c.resp.Result)
}

func (c *sendMessageCallMetadata) assertInvalidMessageError(t *testing.T, message string) {
	require.Error(t, c.err)
	require.Nil(t, c.resp)

	status, ok := status.FromError(c.err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, status.Code())
	assert.True(t, strings.Contains(strings.ToLower(status.Message()), strings.ToLower(message)))
}

func (c *sendMessageCallMetadata) assertUnauthenticatedError(t *testing.T, message string) {
	require.Error(t, c.err)
	require.Nil(t, c.resp)

	status, ok := status.FromError(c.err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, status.Code())
	assert.True(t, strings.Contains(strings.ToLower(status.Message()), strings.ToLower(message)))
}

func (c *clientEnv) sendRequestToGrabBillMessage(t *testing.T, rendezvousKey *common.Account) *sendMessageCallMetadata {
	destination := testutil.NewRandomAccount(t)

	ownerAccount := testutil.NewRandomAccount(t)
	accountInfoRecord := &account.Record{
		OwnerAccount:     ownerAccount.PublicKey().ToBase58(),
		AuthorityAccount: ownerAccount.PublicKey().ToBase58(),
		TokenAccount:     destination.PublicKey().ToBase58(),
		MintAccount:      common.CoreMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
		Index:            0,
	}
	if c.conf.simulateInvalidAccountType {
		accountInfoRecord.AuthorityAccount = testutil.NewRandomAccount(t).PublicKey().ToBase58()
		accountInfoRecord.AccountType = commonpb.AccountType_TEMPORARY_INCOMING
	}
	if !c.conf.simulateAccountNotCodeAccount {
		require.NoError(t, c.directDataAccess.CreateAccountInfo(c.ctx, accountInfoRecord))
	}

	req := &messagingpb.SendMessageRequest{
		Message: &messagingpb.Message{
			Kind: &messagingpb.Message_RequestToGrabBill{
				RequestToGrabBill: &messagingpb.RequestToGrabBill{
					RequestorAccount: destination.ToProto(),
				},
			},
		},
		RendezvousKey: &messagingpb.RendezvousKey{
			Value: rendezvousKey.PublicKey().ToBytes(),
		},
	}

	return c.sendMessage(t, req, rendezvousKey)
}

func (c *clientEnv) sendMessage(t *testing.T, req *messagingpb.SendMessageRequest, rendezvousKey *common.Account) *sendMessageCallMetadata {
	messageBytes, err := proto.Marshal(req.Message)
	require.NoError(t, err)

	signer := rendezvousKey
	if c.conf.simulateInvalidRequestSignature {
		signer = testutil.NewRandomAccount(t)
	}

	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(signer.PrivateKey().ToBytes(), messageBytes),
	}

	resp, err := c.client.SendMessage(c.ctx, req)
	return &sendMessageCallMetadata{
		req:  req,
		resp: resp,
		err:  err,
	}
}

func (c *clientEnv) ackMessages(t *testing.T, rendezvousKey *common.Account, ids ...*messagingpb.MessageId) {
	resp, err := c.client.AckMessages(c.ctx, &messagingpb.AckMessagesRequest{
		RendezvousKey: &messagingpb.RendezvousKey{
			Value: rendezvousKey.PublicKey().ToBytes(),
		},
		MessageIds: ids,
	})
	require.NoError(t, err)
	assert.Equal(t, messagingpb.AckMesssagesResponse_OK, resp.Result)
}

func (c *clientEnv) resetConf() {
	c.conf = &clientConf{}
}
