package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2/tests"
)

func TestInviteV2MemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}

	tests.RunTests(t, testStore, teardown)
}
