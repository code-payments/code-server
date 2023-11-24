package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/push/tests"
)

func TestPushMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
