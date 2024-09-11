package tests

import (
	"testing"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v2"
)

func RunTests(t *testing.T, s chat.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s chat.Store){} {
		tf(t, s)
		teardown()
	}
}
