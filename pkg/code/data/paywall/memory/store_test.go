package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/paywall/tests"
)

func TestPaywallMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
