package chat

import (
	"context"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v2"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
)

type Notifier interface {
	NotifyMessage(ctx context.Context, chatID chat.ChatId, message *chatpb.ChatMessage)
}

type NoopNotifier struct{}

func NewNoopNotifier() *NoopNotifier {
	return &NoopNotifier{}
}

func (n *NoopNotifier) NotifyMessage(_ context.Context, _ chat.ChatId, _ *chatpb.ChatMessage) {
}
