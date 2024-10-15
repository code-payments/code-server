package chat_v2

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	// todo: configurable
	streamBufferSize           = 64
	streamPingDelay            = 5 * time.Second
	streamKeepAliveRecvTimeout = 10 * time.Second
	streamNotifyTimeout        = time.Second
)

type eventStream interface {
	notify(notification *chatEventNotification, timeout time.Duration) error
}

type protoEventStream[T proto.Message] struct {
	sync.Mutex

	closed    bool
	ch        chan T
	transform func(*chatEventNotification) (T, bool)
}

func newEventStream[T proto.Message](
	bufferSize int,
	selector func(notification *chatEventNotification) (T, bool),
) *protoEventStream[T] {
	return &protoEventStream[T]{
		ch:        make(chan T, bufferSize),
		transform: selector,
	}
}

func (e *protoEventStream[T]) notify(event *chatEventNotification, timeout time.Duration) error {
	msg, ok := e.transform(event)
	if !ok {
		return nil
	}

	e.Lock()
	if e.closed {
		e.Unlock()
		return errors.New("cannot notify closed stream")
	}

	select {
	case e.ch <- msg:
	case <-time.After(timeout):
		e.Unlock()
		e.close()
		return errors.New("timed out sending message to streamCh")
	}

	e.Unlock()
	return nil
}

func (e *protoEventStream[T]) close() {
	e.Lock()
	defer e.Unlock()

	if e.closed {
		return
	}

	e.closed = true
	close(e.ch)
}

type ptr[T any] interface {
	proto.Message
	*T
}

func boundedReceive[Req any, ReqPtr ptr[Req]](
	ctx context.Context,
	stream grpc.ServerStream,
	timeout time.Duration,
) (ReqPtr, error) {
	var err error
	var req = new(Req)
	doneCh := make(chan struct{})

	go func() {
		err = stream.RecvMsg(req)
		close(doneCh)
	}()

	select {
	case <-doneCh:
		return req, err
	case <-ctx.Done():
		return req, status.Error(codes.Canceled, "")
	case <-time.After(timeout):
		return req, status.Error(codes.DeadlineExceeded, "timeout receiving message")
	}
}

func monitorStreamHealth[Req any, ReqPtr ptr[Req]](
	ctx context.Context,
	log *logrus.Entry,
	ssRef string,
	streamer grpc.ServerStream,
	validFn func(ReqPtr) bool,
) <-chan struct{} {
	healthCh := make(chan struct{})
	go func() {
		defer close(healthCh)

		for {
			req, err := boundedReceive[Req, ReqPtr](ctx, streamer, streamKeepAliveRecvTimeout)
			if err != nil {
				return
			}

			if !validFn(req) {
				return
			}
			log.Tracef("received pong from client (stream=%s)", ssRef)
		}
	}()
	return healthCh
}
