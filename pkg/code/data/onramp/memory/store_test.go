package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/onramp/tests"
)

func TestOnrampMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
