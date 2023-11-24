package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/account/tests"
)

func TestAccountInfoMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
