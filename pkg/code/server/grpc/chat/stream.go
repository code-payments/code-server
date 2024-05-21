package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/chat"
)

const (
	// todo: configurable
	streamBufferSize           = 64
	streamPingDelay            = 5 * time.Second
	streamKeepAliveRecvTimeout = 10 * time.Second
	streamNotifyTimeout        = time.Second
)

type chatEventStream struct {
	sync.Mutex

	closed   bool
	streamCh chan *chatpb.ChatStreamEvent
}

func newChatEventStream(bufferSize int) *chatEventStream {
	return &chatEventStream{
		streamCh: make(chan *chatpb.ChatStreamEvent, bufferSize),
	}
}

func (s *chatEventStream) notify(event *chatpb.ChatStreamEvent, timeout time.Duration) error {
	m := proto.Clone(event).(*chatpb.ChatStreamEvent)

	s.Lock()

	if s.closed {
		s.Unlock()
		return errors.New("cannot notify closed stream")
	}

	select {
	case s.streamCh <- m:
	case <-time.After(timeout):
		s.Unlock()
		s.close()
		return errors.New("timed out sending message to streamCh")
	}

	s.Unlock()
	return nil
}

func (s *chatEventStream) close() {
	s.Lock()
	defer s.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	close(s.streamCh)
}

func boundedStreamChatEventsRecv(
	ctx context.Context,
	streamer chatpb.Chat_StreamChatEventsServer,
	timeout time.Duration,
) (req *chatpb.StreamChatEventsRequest, err error) {
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

type chatEventNotification struct {
	chatId chat.ChatId
	owner  *common.Account
	event  *chatpb.ChatStreamEvent
	ts     time.Time
}

func (s *server) asyncNotifyAll(chatId chat.ChatId, owner *common.Account, event *chatpb.ChatStreamEvent) error {
	m := proto.Clone(event).(*chatpb.ChatStreamEvent)
	ok := s.chatEventChans.Send(chatId[:], &chatEventNotification{chatId, owner, m, time.Now()})
	if !ok {
		return errors.New("chat event channel is full")
	}
	return nil
}

func (s *server) asyncChatEventStreamNotifier(workerId int, channel <-chan interface{}) {
	log := s.log.WithFields(logrus.Fields{
		"method": "asyncChatEventStreamNotifier",
		"worker": workerId,
	})

	for value := range channel {
		typedValue, ok := value.(*chatEventNotification)
		if !ok {
			log.Warn("channel did not receive expected struct")
			continue
		}

		log := log.WithField("chat_id", typedValue.chatId.String())

		if time.Since(typedValue.ts) > time.Second {
			log.Warn("")
		}

		s.streamsMu.RLock()
		for key, stream := range s.streams {
			if !strings.HasPrefix(key, typedValue.chatId.String()) {
				continue
			}

			if strings.HasSuffix(key, typedValue.owner.PublicKey().ToBase58()) {
				continue
			}

			if err := stream.notify(typedValue.event, streamNotifyTimeout); err != nil {
				log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
			}
		}
		s.streamsMu.RUnlock()
	}
}

// Very naive implementation to start
func monitorChatEventStreamHealth(
	ctx context.Context,
	log *logrus.Entry,
	ssRef string,
	streamer chatpb.Chat_StreamChatEventsServer,
) <-chan struct{} {
	streamHealthChan := make(chan struct{})
	go func() {
		defer close(streamHealthChan)

		for {
			// todo: configurable timeout
			req, err := boundedStreamChatEventsRecv(ctx, streamer, streamKeepAliveRecvTimeout)
			if err != nil {
				return
			}

			switch req.Type.(type) {
			case *chatpb.StreamChatEventsRequest_Pong:
				log.Tracef("received pong from client (stream=%s)", ssRef)
			default:
				// Client sent something unexpected. Terminate the stream
				return
			}
		}
	}()
	return streamHealthChan
}
