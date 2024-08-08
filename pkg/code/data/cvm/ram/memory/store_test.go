package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/cvm/ram/tests"
)

func TestVmRamMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
