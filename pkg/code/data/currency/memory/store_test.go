package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/code/data/currency/tests"
)

func TestCurrencyMemoryStore(t *testing.T) {
	testStore := New()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
