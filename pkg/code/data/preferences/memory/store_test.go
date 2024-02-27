package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/preferences/tests"
)

func TestPreferencesMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
