package messaging

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
)

type messageStream struct {
	sync.Mutex

	closed   bool
	streamCh chan *messagingpb.Message
}

func newMessageStream(bufferSize int) *messageStream {
	return &messageStream{
		streamCh: make(chan *messagingpb.Message, bufferSize),
	}
}

func (s *messageStream) notify(msg *messagingpb.Message, timeout time.Duration) error {
	m := proto.Clone(msg).(*messagingpb.Message)

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

func (s *messageStream) close() {
	s.Lock()
	defer s.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	close(s.streamCh)
}
