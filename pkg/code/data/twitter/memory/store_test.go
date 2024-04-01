package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/twitter/tests"
)

func TestTwitterMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
