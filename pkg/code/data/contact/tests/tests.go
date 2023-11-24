package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/contact"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

func RunTests(t *testing.T, s contact.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s contact.Store){
		testHappyPath,
		testHappyPathForBatchCalls,
		testContractRetrievalPaging,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s contact.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		owner := user.NewDataContainerID()
		contact := "+11234567890"

		actual, _, err := s.Get(ctx, owner, 100, nil)
		require.NoError(t, err)
		assert.Empty(t, actual)

		require.NoError(t, s.Add(ctx, owner, contact))
		require.NoError(t, s.Add(ctx, owner, contact))

		actual, _, err = s.Get(ctx, owner, 100, nil)
		require.NoError(t, err)
		require.Len(t, actual, 1)
		assert.Equal(t, actual[0], contact)

		require.NoError(t, s.Remove(ctx, owner, contact))
		require.NoError(t, s.Remove(ctx, owner, contact))

		actual, _, err = s.Get(ctx, owner, 100, nil)
		require.NoError(t, err)
		assert.Empty(t, actual)
	})
}

func testHappyPathForBatchCalls(t *testing.T, s contact.Store) {
	t.Run("testHappyPathForBatchCalls", func(t *testing.T) {
		ctx := context.Background()

		owner := user.NewDataContainerID()
		contacts := make([]string, 0)
		for i := 0; i < 10; i++ {
			contacts = append(contacts, fmt.Sprintf("+1800555000%d", i))
		}

		actual, _, err := s.Get(ctx, owner, 100, nil)
		require.NoError(t, err)
		assert.Empty(t, actual)

		require.NoError(t, s.BatchAdd(ctx, owner, contacts[:len(contacts)/2]))
		require.NoError(t, s.BatchAdd(ctx, owner, contacts))

		actual, _, err = s.Get(ctx, owner, 100, nil)
		require.NoError(t, err)
		require.Len(t, actual, len(contacts))
		assert.Equal(t, contacts, actual)

		require.NoError(t, s.BatchRemove(ctx, owner, contacts[:len(contacts)/2]))
		require.NoError(t, s.BatchRemove(ctx, owner, contacts))

		actual, _, err = s.Get(ctx, owner, 10, nil)
		require.NoError(t, err)
		assert.Empty(t, actual)
	})
}

func testContractRetrievalPaging(t *testing.T, s contact.Store) {
	t.Run("testContractRetrievalPaging", func(t *testing.T) {
		ctx := context.Background()

		owner := user.NewDataContainerID()

		contacts := make([]string, 0)
		for i := 0; i < 10; i++ {
			contacts = append(contacts, fmt.Sprintf("+1800555000%d", i))
			require.NoError(t, s.Add(ctx, owner, contacts[i]))
		}

		for pageSize := 1; pageSize <= len(contacts); pageSize++ {
			var currentPageToken []byte
			var callCount int

			var actual []string
			for {
				callCount++

				if callCount > len(contacts)/pageSize+1 {
					assert.Fail(t, "exceeded maximum call count")
				}

				subset, nextPageToken, err := s.Get(ctx, owner, uint32(pageSize), currentPageToken)
				require.NoError(t, err)

				actual = append(actual, subset...)

				if len(nextPageToken) == 0 {
					assert.True(t, len(subset) <= pageSize)
					break
				}

				assert.Len(t, subset, pageSize)

				currentPageToken = nextPageToken
			}

			require.Len(t, actual, len(contacts))
			assert.Equal(t, contacts, actual)
		}
	})
}
