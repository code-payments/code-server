package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/messaging"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	// todo: configurable
	messageStreamBufferSize              = 64
	messageStreamPingDelay               = 5 * time.Second
	messageStreamKeepAliveRecvTimeout    = 10 * time.Second
	messageStreamWithKeepAliveTimeout    = 15 * time.Minute
	messageStreamWithoutKeepAliveTimeout = time.Minute
	notifyTimeout                        = 10 * time.Second
	rendezvousRecordMaxAge               = messageStreamWithoutKeepAliveTimeout
)

type server struct {
	log  *logrus.Entry
	conf *conf
	data code_data.Provider

	streamsMu          sync.RWMutex
	streams            map[string]*messageStream
	individualStreamMu map[string]*sync.Mutex

	domainVerifier thirdparty.DomainVerifier

	rendezvousFirstSeenAtCache cache.Cache // todo: Back with something like Redis when we go multi-server

	rpcSignatureVerifier *auth.RPCSignatureVerifier

	broadcastAddress string

	messagingpb.UnimplementedMessagingServer
}

// NewMessagingClient returns a new internal messaging client
//
// todo: Proper separation of internal client and server
func NewMessagingClient(
	data code_data.Provider,
) InternalMessageClient {
	return &server{
		log:  logrus.StandardLogger().WithField("type", "messaging/client"),
		data: data,
	}
}

// NewMessagingClientAndServer returns a new messaging client and server bundle.
//
// These are currently highly coupled atm due to need to detect an active stream
// on a local server and avoiding the network call.
//
// Note: The multi-server implementation of this server is not perfect, and it
// doesn't need to be initially. We're mostly acting as a notification system
// where updates can be sent out-of-order. If we need stronger guarantees,
// resurrecting the Black Marlin KikX project might be necessary. Ideally we'd
// avoid this due to the step level increase in complexity.
//
// todo: Proper separation of internal client and server
func NewMessagingClientAndServer(
	data code_data.Provider,
	rpcSignatureVerifier *auth.RPCSignatureVerifier,
	broadcastAddress string,
	configProvider ConfigProvider,
) *server {
	return &server{
		log:                        logrus.StandardLogger().WithField("type", "messaging/client_and_server"),
		conf:                       configProvider(),
		data:                       data,
		streams:                    make(map[string]*messageStream),
		individualStreamMu:         make(map[string]*sync.Mutex),
		domainVerifier:             thirdparty.VerifyDomainNameOwnership,
		rendezvousFirstSeenAtCache: cache.NewCache(100_000),
		rpcSignatureVerifier:       rpcSignatureVerifier,
		broadcastAddress:           broadcastAddress,
	}
}

// OpenMessageStreamWithKeepAlive implements messagingpb.MessagingServer.OpenMessageStreamWithKeepAlive.
//
// todo: Majority of message streaming logic is duplicated here and in OpenMessageStream
func (s *server) OpenMessageStreamWithKeepAlive(streamer messagingpb.Messaging_OpenMessageStreamWithKeepAliveServer) error {
	ctx := streamer.Context()

	req, err := s.boundedRecv(ctx, streamer, 250*time.Millisecond)
	if err != nil {
		return err
	}

	if req.GetRequest() == nil {
		return status.Error(codes.InvalidArgument, "request is nil")
	}

	if req.GetRequest().Signature == nil {
		return status.Error(codes.InvalidArgument, "signature is nil")
	}

	streamKey := base58.Encode(req.GetRequest().RendezvousKey.Value)

	log := s.log.WithFields(logrus.Fields{
		"method":         "OpenMessageStreamWithKeepAlive",
		"rendezvous_key": streamKey,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	rendezvousAccount, err := common.NewAccountFromPublicKeyString(streamKey)
	if err != nil {
		log.WithError(err).Warn("rendezvous key isn't a valid public key")
		return status.Error(codes.Internal, "")
	}

	s.markRendezvousKeyAsSeen(rendezvousAccount)

	signature := req.GetRequest().Signature
	req.GetRequest().Signature = nil
	if err = s.rpcSignatureVerifier.Authenticate(streamer.Context(), rendezvousAccount, req.GetRequest(), signature); err != nil {
		return err
	}

	s.streamsMu.Lock()

	ms, exists := s.streams[streamKey]
	if exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this server (stream=%p) ; aborting", ms)
		return status.Error(codes.Aborted, "stream already exists")
	}

	ms = newMessageStream(messageStreamBufferSize)
	log.Tracef("setting up new stream (stream=%p)", ms)
	s.streams[streamKey] = ms

	myStreamMu, ok := s.individualStreamMu[streamKey]
	if !ok {
		myStreamMu = &sync.Mutex{}
		s.individualStreamMu[streamKey] = myStreamMu
	}

	// The race detector complains when reading the stream pointer ref outside of the lock.
	ssRef := fmt.Sprintf("%p", ms)

	s.streamsMu.Unlock()

	timeoutChan := time.After(messageStreamWithKeepAliveTimeout + time.Second)

	// Distribute lock the stream, so there's a single endpoint processing
	// real time message updates. In order to accomplish this, we need to
	// guarantee consistent management of the rendezvous record.
	//
	// todo: A real distributed lock. This approach will leak memory, but is
	//       necessary since a striped lock could cause cross-payment delays
	//       due to the long-lived nature of a stream.
	// todo: This can potentially hang for a really long time. This isn't a
	//       problem until we're actually multi-server and we have a real
	//       distributed lock where we can timeout the lock acquire process.
	myStreamMu.Lock()

	defer func() {
		s.streamsMu.Lock()

		log.Tracef("closing streamer (stream=%s)", ssRef)

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another OpenMessageStream() call is handling it.
		liveStream, exists := s.streams[streamKey]
		if exists && liveStream == ms {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()

		// Delete the rendezvous record after killing the stream. This will allow
		// another stream to "queue up" on the same server without failing with a
		// duplication check while we wait for this slower DB operation.
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		err := s.data.DeleteRendezvous(ctx, streamKey)
		if err != nil {
			log.WithError(err).Warn("failed to cleanup rendezvous record")
		}
		cancel()

		myStreamMu.Unlock()
	}()

	// Sanity check whether the stream is still valid before doing expensive operations
	select {
	case <-timeoutChan:
		log.Tracef("stream timed out ; ending stream (stream=%s)", ssRef)
		return status.Error(codes.DeadlineExceeded, "")
	case <-ctx.Done():
		log.Tracef("stream context cancelled ; ending stream (stream=%s)", ssRef)
		return status.Error(codes.Canceled, "")
	default:
	}

	// Let other RPC servers know where to find the active stream
	rendezvousRecord := &rendezvous.Record{
		Key:           streamKey,
		Location:      s.broadcastAddress,
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
	}
	err = s.data.SaveRendezvous(ctx, rendezvousRecord)
	if err != nil {
		log.WithError(err).Warn("failure saving rendezvous record")
		return status.Error(codes.Internal, "")
	}

	// Since ordering doesn't matter, we can async flush QoS. Importantly, this
	// must occur after setting the rendezvous DB record to avoid race conditions
	// with the internal forwarding logic.
	go s.flush(ctx, req.GetRequest().RendezvousKey, ms)

	sendPingCh := time.After(0)
	streamHealthCh := s.monitorOpenMessageStreamHealth(ctx, log, ssRef, streamer)
	updateRendezvousRecordCh := time.After(3 * rendezvousRecordMaxAge / 4)

	for {
		select {
		case msg, ok := <-ms.streamCh:
			if !ok {
				log.Tracef("message stream closed ; ending stream (stream=%s)", ssRef)
				return status.Error(codes.Aborted, "message stream closed")
			}

			err := streamer.Send(&messagingpb.OpenMessageStreamWithKeepAliveResponse{
				ResponseOrPing: &messagingpb.OpenMessageStreamWithKeepAliveResponse_Response{
					Response: &messagingpb.OpenMessageStreamResponse{
						Messages: []*messagingpb.Message{msg},
					},
				},
			})
			if err != nil {
				log.WithError(err).Info("failed to forward message")
				return err
			}
		case <-updateRendezvousRecordCh:
			log.Tracef("refreshing rendezvous record (stream=%s)", ssRef)

			updateRendezvousRecordCh = time.After(3 * rendezvousRecordMaxAge / 4)

			rendezvousRecord.LastUpdatedAt = time.Now()
			err = s.data.SaveRendezvous(ctx, rendezvousRecord)
			if err != nil {
				log.WithError(err).Warn("failure refreshing rendezvous record")
				return status.Error(codes.Internal, "")
			}
		case <-sendPingCh:
			log.Tracef("sending ping to client (stream=%s)", ssRef)

			sendPingCh = time.After(messageStreamPingDelay)

			err := streamer.Send(&messagingpb.OpenMessageStreamWithKeepAliveResponse{
				ResponseOrPing: &messagingpb.OpenMessageStreamWithKeepAliveResponse_Ping{
					Ping: &commonpb.ServerPing{
						Timestamp: timestamppb.Now(),
						PingDelay: durationpb.New(messageStreamPingDelay),
					},
				},
			})
			if err != nil {
				log.Tracef("stream is unhealthy ; aborting (stream=%s)", ssRef)
				return status.Error(codes.Aborted, "terminating unhealthy stream")
			}
		case <-streamHealthCh:
			log.Tracef("stream is unhealthy ; aborting (stream=%s)", ssRef)
			return status.Error(codes.Aborted, "terminating unhealthy stream")
		case <-timeoutChan:
			log.Tracef("stream timed out ; ending stream (stream=%s)", ssRef)
			return status.Error(codes.DeadlineExceeded, "")
		case <-ctx.Done():
			log.Tracef("stream context cancelled ; ending stream (stream=%s)", ssRef)
			return status.Error(codes.Canceled, "")
		}
	}
}

func (s *server) boundedRecv(
	ctx context.Context,
	streamer messagingpb.Messaging_OpenMessageStreamWithKeepAliveServer,
	timeout time.Duration,
) (req *messagingpb.OpenMessageStreamWithKeepAliveRequest, err error) {
	done := make(chan struct{})
	go func() {
		req, err = streamer.Recv()
		close(done)
	}()

	select {
	case <-done:
		return req, err
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, "")
	case <-time.After(timeout):
		return nil, status.Error(codes.DeadlineExceeded, "timed out receiving message")
	}
}

// Very naive implementation to start
func (s *server) monitorOpenMessageStreamHealth(
	ctx context.Context,
	log *logrus.Entry,
	ssRef string,
	streamer messagingpb.Messaging_OpenMessageStreamWithKeepAliveServer,
) <-chan struct{} {
	streamHealthChan := make(chan struct{})
	go func() {
		defer close(streamHealthChan)

		for {
			req, err := s.boundedRecv(ctx, streamer, messageStreamKeepAliveRecvTimeout)
			if err != nil {
				return
			}

			switch req.RequestOrPong.(type) {
			case *messagingpb.OpenMessageStreamWithKeepAliveRequest_Pong:
				log.Tracef("received pong from client (stream=%s)", ssRef)
			default:
				// Client sent something unexpected. Terminate the stream
				return
			}
		}
	}()
	return streamHealthChan
}

// OpenMessageStream implements messagingpb.MessagingServer.OpenMessageStream.
//
// Note: This variant is more suitable for short-lived streams, and is coded as
// such by having a hard upper bound time that it can be opened.
//
// todo: Majority of message streaming logic is duplicated here and in OpenMessageStream
func (s *server) OpenMessageStream(req *messagingpb.OpenMessageStreamRequest, streamer messagingpb.Messaging_OpenMessageStreamServer) error {
	ctx := streamer.Context()

	streamKey := base58.Encode(req.RendezvousKey.Value)

	log := s.log.WithFields(logrus.Fields{
		"method":         "OpenMessageStream",
		"rendezvous_key": streamKey,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	rendezvousAccount, err := common.NewAccountFromPublicKeyString(streamKey)
	if err != nil {
		log.WithError(err).Warn("rendezvous key isn't a valid public key")
		return status.Error(codes.Internal, "")
	}

	s.markRendezvousKeyAsSeen(rendezvousAccount)

	if req.Signature != nil {
		signature := req.Signature
		req.Signature = nil
		if err = s.rpcSignatureVerifier.Authenticate(ctx, rendezvousAccount, req, signature); err != nil {
			return err
		}
	}

	s.streamsMu.Lock()

	ms, exists := s.streams[streamKey]
	if exists {
		s.streamsMu.Unlock()
		// There's an existing stream on this server that must be terminated first.
		// Warn to see how often this happens in practice
		log.Warnf("existing stream detected on this server (stream=%p) ; aborting", ms)
		return status.Error(codes.Aborted, "stream already exists")
	}

	ms = newMessageStream(messageStreamBufferSize)
	log.Tracef("setting up new stream (stream=%p)", ms)
	s.streams[streamKey] = ms

	myStreamMu, ok := s.individualStreamMu[streamKey]
	if !ok {
		myStreamMu = &sync.Mutex{}
		s.individualStreamMu[streamKey] = myStreamMu
	}

	// The race detector complains when reading the stream pointer ref outside of the lock.
	ssRef := fmt.Sprintf("%p", ms)

	s.streamsMu.Unlock()

	timeoutChan := time.After(messageStreamWithoutKeepAliveTimeout + time.Second)

	// Distribute lock the stream, so there's a single endpoint processing
	// real time message updates. In order to accomplish this, we need to
	// guarantee consistent management of the rendezvous record.
	//
	// todo: A real distributed lock. This approach will leak memory, but is
	//       necessary since a striped lock could cause cross-payment delays
	//       due to the long-lived nature of a stream.
	// todo: This can potentially hang for 60s. This isn't a problem until
	//       we're actually multi-server and we have a real distributed lock
	//       where we can timeout the lock acquire process.
	myStreamMu.Lock()

	defer func() {
		s.streamsMu.Lock()

		log.Tracef("closing streamer (stream=%s)", ssRef)

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another OpenMessageStream() call is handling it.
		liveStream, exists := s.streams[streamKey]
		if exists && liveStream == ms {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()

		// Delete the rendezvous record after killing the stream. This will allow
		// another stream to "queue up" on the same server without failing with a
		// duplication check while we wait for this slower DB operation.
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		err := s.data.DeleteRendezvous(ctx, streamKey)
		if err != nil {
			log.WithError(err).Warn("failed to cleanup rendezvous record")
		}
		cancel()

		myStreamMu.Unlock()
	}()

	// Sanity check whether the stream is still valid before doing expensive operations
	select {
	case <-timeoutChan:
		log.Tracef("stream timed out ; ending stream (stream=%s)", ssRef)
		return status.Error(codes.DeadlineExceeded, "")
	case <-streamer.Context().Done():
		log.Tracef("stream context cancelled ; ending stream (stream=%s)", ssRef)
		return status.Error(codes.Canceled, "")
	default:
	}

	// Let other RPC servers know where to find the active stream
	rendezvousRecord := &rendezvous.Record{
		Key:           streamKey,
		Location:      s.broadcastAddress,
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
	}
	err = s.data.SaveRendezvous(ctx, rendezvousRecord)
	if err != nil {
		log.WithError(err).Warn("failure saving rendezvous record")
		return status.Error(codes.Internal, "")
	}

	// Since ordering doesn't matter, we can async flush QoS. Importantly, this
	// must occur after setting the rendezvous DB record to avoid race conditions
	// with the internal forwarding logic.
	go s.flush(ctx, req.RendezvousKey, ms)

	for {
		select {
		case msg, ok := <-ms.streamCh:
			if !ok {
				log.Tracef("message stream closed ; ending stream (stream=%s)", ssRef)
				return status.Error(codes.Aborted, "")
			}

			// Currently only support streams that have short lifespans in terms
			// of key validity and length of time opened. Current implementation
			// might be ok, but we should be thoughtful of any additional edge cases
			// that might arise.
			switch msg.Kind.(type) {
			case *messagingpb.Message_AirdropReceived:
				continue
			}

			err := streamer.Send(&messagingpb.OpenMessageStreamResponse{
				Messages: []*messagingpb.Message{msg},
			})
			if err != nil {
				log.WithError(err).Info("failed to forward message")
				return err
			}
		case <-timeoutChan:
			log.Tracef("stream timed out ; ending stream (stream=%s)", ssRef)
			return status.Error(codes.DeadlineExceeded, "")
		case <-ctx.Done():
			log.Tracef("stream context cancelled ; ending stream (stream=%s)", ssRef)
			return status.Error(codes.Canceled, "")
		}
	}
}

// PollMessages implements messagingpb.MessagingServer.PollMessages.
func (s *server) PollMessages(ctx context.Context, req *messagingpb.PollMessagesRequest) (*messagingpb.PollMessagesResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":         "PollMessages",
		"rendezvous_key": base58.Encode(req.RendezvousKey.Value),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	rendezvousAccount, err := common.NewAccountFromPublicKeyString(base58.Encode(req.RendezvousKey.Value))
	if err != nil {
		log.WithError(err).Warn("rendezvous key isn't a valid public key")
		return nil, status.Error(codes.Internal, "")
	}

	s.markRendezvousKeyAsSeen(rendezvousAccount)

	signature := req.Signature
	req.Signature = nil
	if err = s.rpcSignatureVerifier.Authenticate(ctx, rendezvousAccount, req, signature); err != nil {
		return nil, err
	}

	records, err := s.data.GetMessages(ctx, rendezvousAccount.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failed to load undelivered messages")
		return nil, status.Error(codes.Internal, "")
	}

	var messages []*messagingpb.Message
	for _, r := range records {
		var message messagingpb.Message
		if err := proto.Unmarshal(r.Message, &message); err != nil {
			// todo(safety): this is the equivalent QoS brick case, although should be less problematic.
			//               we could have a valve to ignore, and also to delete
			log.WithError(err).Warn("Failed to unmarshal message bytes")
			return nil, status.Error(codes.Internal, "")
		}

		messages = append(messages, &message)

		// Upper bound messages transmitted. Clients need to ack and cleanup. A
		// typical short-lived stream should have a small number of messages
		// anyways.
		if len(messages) > 256 {
			break
		}
	}

	return &messagingpb.PollMessagesResponse{
		Messages: messages,
	}, nil
}

// AckMessages implements messagingpb.MessagingServer.AckMessages.
func (s *server) AckMessages(ctx context.Context, req *messagingpb.AckMessagesRequest) (*messagingpb.AckMesssagesResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "AckMessages",
		"acks":   len(req.MessageIds),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	account := base58.Encode(req.RendezvousKey.Value)

	log = log.WithField("account_id", account)

	// todo(perf): support batch deletes?
	for _, id := range req.MessageIds {
		converted, err := uuid.FromBytes(id.Value)
		if err != nil {
			log.WithError(err).Warn("Failed to convert message ID bytes to UUID")
			return nil, status.Error(codes.Internal, "")
		}

		if err := s.data.DeleteMessage(ctx, account, converted); err != nil {
			log.WithError(err).Warn("Failed to delete message")
			return nil, status.Error(codes.Internal, "")
		}
	}

	return &messagingpb.AckMesssagesResponse{}, nil
}

// SendMessage implements messagingpb.MessagingServer.SendMessage.
func (s *server) SendMessage(ctx context.Context, req *messagingpb.SendMessageRequest) (*messagingpb.SendMessageResponse, error) {
	streamKey := base58.Encode(req.RendezvousKey.Value)

	log := s.log.WithFields(logrus.Fields{
		"method":         "SendMessage",
		"rendezvous_key": streamKey,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	rendezvousAccount, err := common.NewAccountFromPublicKeyString(streamKey)
	if err != nil {
		log.WithError(err).Warn("rendezvous key isn't a valid public key")
		return nil, status.Error(codes.Internal, "")
	}

	// The request message has a message ID, which implies it was forwarded by
	// another RPC server. All we need to do is to verify the request, and attempt
	// to forward it to receiver's open message stream, if it exists.
	//
	// todo: Long term, we need public and internal APIs properly separated. For now,
	//       we'll sign things that look internal to verify.
	if req.Message.Id != nil {
		verified, err := s.verifyForwardedSendMessageRequest(ctx, req)
		if err != nil {
			log.WithError(err).Warn("failure verifying if message was internally forwarded")
			return nil, status.Error(codes.Internal, "")
		} else if !verified {
			return nil, status.Error(codes.InvalidArgument, "message.id cannot be set by clients")
		}

		s.streamsMu.RLock()
		stream := s.streams[streamKey]
		s.streamsMu.RUnlock()

		if stream != nil {
			if err := stream.notify(req.Message, notifyTimeout); err != nil {
				log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
			}
		}

		// Always return OK regardless if the stream is the latest or whether it
		// exists. Anything newer stream will always pick up the message because
		// of the flush because we saved it to the DB before forwarding it.
		return &messagingpb.SendMessageResponse{
			Result:    messagingpb.SendMessageResponse_OK,
			MessageId: req.Message.Id,
		}, nil
	}

	// Otherwise, handle the request as a brand new message that must both be
	// created and sent to the receiver's stream, possibly by forwarding it to
	// another RPC server.

	if req.Message.SendMessageRequestSignature != nil {
		return nil, status.Error(codes.InvalidArgument, "message.send_message_request_signature cannot be set by clients")
	}

	var messageHandler MessageHandler
	switch req.Message.Kind.(type) {

	//
	// Section: Cash
	//

	case *messagingpb.Message_RequestToGrabBill:
		log = log.WithField("message_type", "request_to_grab_bill")
		messageHandler = NewRequestToGrabBillMessageHandler(s.data)

	//
	// Section: Payment Request
	//

	case *messagingpb.Message_RequestToReceiveBill:
		log = log.WithField("message_type", "request_to_receive_bill")
		messageHandler = NewRequestToReceiveBillMessageHandler(s.conf, s.data, s.rpcSignatureVerifier, s.domainVerifier)
	case *messagingpb.Message_ClientRejectedPayment:
		log = log.WithField("message_type", "client_rejected_payment")
		messageHandler = NewClientRejectedPaymentMessageHandler()
	case *messagingpb.Message_CodeScanned:
		log = log.WithField("message_type", "code_scanned")
		messageHandler = NewCodeScannedMessageHandler()
	case *messagingpb.Message_IntentSubmitted:
		return nil, status.Error(codes.InvalidArgument, "message.kind cannot be intent_submitted")
	case *messagingpb.Message_WebhookCalled:
		return nil, status.Error(codes.InvalidArgument, "message.kind cannot be webhook_called")

	//
	// Section: Login
	//

	case *messagingpb.Message_RequestToLogin:
		log = log.WithField("message_type", "request_to_login")
		messageHandler = NewRequestToLoginMessageHandler(s.data, s.rpcSignatureVerifier, s.domainVerifier)
	case *messagingpb.Message_ClientRejectedLogin:
		log = log.WithField("message_type", "client_rejected_login")
		messageHandler = NewClientRejectedLoginMessageHandler()

	//
	// Section: Airdrops
	//

	case *messagingpb.Message_AirdropReceived:
		return nil, status.Error(codes.InvalidArgument, "message.kind cannot be airdrop_received")

	default:
		return nil, status.Error(codes.InvalidArgument, "message.kind must be set")
	}

	if err := s.rpcSignatureVerifier.Authenticate(ctx, rendezvousAccount, req.Message, req.Signature); err != nil {
		return nil, err
	}

	err = messageHandler.Validate(ctx, rendezvousAccount, req.Message)
	if err != nil {
		switch err.(type) {
		case MessageValidationError:
			log.WithError(err).Warn("client sent an invalid message")
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case MessageAuthenticationError:
			return nil, status.Error(codes.Unauthenticated, err.Error())
		case MessageAuthorizationError:
			return nil, status.Error(codes.PermissionDenied, err.Error())
		default:
			log.WithError(err).Warn("failure validating message")
			return nil, status.Error(codes.Internal, "")
		}
	}

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	req.Message.Id = &messagingpb.MessageId{
		Value: idBytes,
	}
	req.Message.SendMessageRequestSignature = req.Signature

	messageWithGeneratedIDAndSignatureBytes, err := proto.Marshal(req.Message)
	if err != nil {
		log.WithError(err).Warn("Failed to marshal message")
		return nil, status.Error(codes.Internal, err.Error())
	}

	if ok, maxDuration := messageHandler.RequiresActiveStream(); ok {
		// Is there an active stream? If not, do we have any reason to believe one
		// may come avaliable?
		_, err = s.data.GetRendezvous(ctx, rendezvousAccount.PublicKey().ToBase58())
		if err == rendezvous.ErrNotFound {
			// There is no active stream, but we need to be cautious to not introduce
			// instability. Only deny the message once we're 100% we've passed the time
			// when the stream is deemed valid, which is inferred by the use case.
			ts, ok := s.rendezvousFirstSeenAtCache.Retrieve(rendezvousAccount.PublicKey().ToBase58())
			if ok && time.Since(ts.(time.Time)) > maxDuration {
				return &messagingpb.SendMessageResponse{
					Result: messagingpb.SendMessageResponse_NO_ACTIVE_STREAM,
				}, nil
			}
			s.markRendezvousKeyAsSeen(rendezvousAccount)
		} else if err != nil {
			log.WithError(err).Warn("failure getting stream status")
			return nil, status.Error(codes.Internal, "")
		}
	}

	// Start off by persisting the message, so any async flushes will catch it.
	// In the same database transaction, store any supporting DB records as
	// required by the message type.
	//
	// Note: Not all store implementations have real support for this, so if
	// anything is added, then ensure it does!
	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		record := &messaging.Record{
			Account:   base58.Encode(req.RendezvousKey.Value),
			MessageID: id,
			Message:   messageWithGeneratedIDAndSignatureBytes,
		}

		err = s.data.CreateMessage(ctx, record)
		if err != nil {
			log.WithError(err).Warn("failed to create message")
			return err
		}

		err = messageHandler.OnSuccess(ctx)
		if err != nil {
			log.WithError(err).Warn("failure calling message hanlder success callback")
			return err
		}

		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	// Next, forward the message to the receiver for a real-time update.
	_, err = retry.Retry(
		func() error {
			return s.internallyForwardMessage(ctx, req)
		},
		retry.Limit(3),
		retry.Backoff(backoff.Constant(100*time.Millisecond), 100*time.Millisecond),
	)
	if err != nil {
		isRpcFailure := true

		internalRpcStatus, ok := status.FromError(err)
		if ok && internalRpcStatus.Code() == codes.Unavailable && strings.Contains(err.Error(), "connection refused") {
			// RPC node cannot be connected to. It might have been deployed,
			// scaled out, crashed, etc. The rendezvous record also hasn't been
			// cleaned up and we're within the stream timeout. This is an edge case,
			// and we won't consider it a failure. It's effectively the same as
			// forwarding it to a server where the stream doesn't exist. The
			// message will be picked up on the next stream open.
			isRpcFailure = false
		}

		if isRpcFailure {
			log.Warn("unable to internally forward the message")
			return nil, status.Error(codes.Internal, "")
		}
	}

	return &messagingpb.SendMessageResponse{
		Result:    messagingpb.SendMessageResponse_OK,
		MessageId: req.Message.Id,
	}, nil
}

func (s *server) flush(ctx context.Context, accountID *messagingpb.RendezvousKey, stream *messageStream) {
	accountStr := base58.Encode(accountID.Value)

	log := s.log.WithFields(logrus.Fields{
		"method":     "flush",
		"account_id": accountStr,
	})

	records, err := s.data.GetMessages(ctx, accountStr)
	if err != nil {
		log.WithError(err).Warn("Failed to load undelivered messages")
		return
	}

	for _, r := range records {
		var message messagingpb.Message
		if err := proto.Unmarshal(r.Message, &message); err != nil {
			// todo(safety): this is the equivalent QoS brick case, although should be less problematic.
			//               we could have a valve to ignore, and also to delete
			log.WithError(err).Warn("Failed to unmarshal message bytes")
			return
		}

		if err := stream.notify(&message, notifyTimeout); err != nil {
			log.WithError(err).Warn("Failed to send undelivered message")
			return
		}
	}
}

func (s *server) markRendezvousKeyAsSeen(rendezvousAccount *common.Account) {
	_, ok := s.rendezvousFirstSeenAtCache.Retrieve(rendezvousAccount.PublicKey().ToBase58())
	if !ok {
		s.rendezvousFirstSeenAtCache.Insert(rendezvousAccount.PublicKey().ToBase58(), time.Now(), 1)
	}
}
