package tests

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/messaging"
)

func RunTests(t *testing.T, s messaging.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s messaging.Store){
		testHappyPath,
		testDuplicateMessage,
		testGetMultipleMessages,
		testDeleteNonExistantMessage,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s messaging.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		record := makeUniqueRecordForAccount("account")

		require.NoError(t, s.Insert(ctx, record))

		actual, err := s.Get(ctx, record.Account)
		require.NoError(t, err)
		require.Len(t, actual, 1)
		assertEquivalentRecords(t, record, actual[0])

		require.NoError(t, s.Delete(ctx, record.Account, record.MessageID))

		actual, err = s.Get(ctx, record.Account)
		require.NoError(t, err)
		assert.Empty(t, actual)
	})
}

func testDuplicateMessage(t *testing.T, s messaging.Store) {
	t.Run("testDuplicateMessage", func(t *testing.T) {
		ctx := context.Background()
		record := makeUniqueRecordForAccount("account")
		require.NoError(t, s.Insert(ctx, record))
		assert.Equal(t, messaging.ErrDuplicateMessageID, s.Insert(ctx, record))
	})
}

func testGetMultipleMessages(t *testing.T, s messaging.Store) {
	t.Run("testGetMultipleMessages", func(t *testing.T) {
		ctx := context.Background()

		records := []*messaging.Record{
			makeUniqueRecordForAccount("account"),
			makeUniqueRecordForAccount("account"),
			makeUniqueRecordForAccount("account"),
		}

		for _, record := range records {
			require.NoError(t, s.Insert(ctx, record))
		}

		actual, err := s.Get(ctx, records[0].Account)
		require.NoError(t, err)
		require.Len(t, actual, len(records))

		for _, record := range records {
			var found bool
			for _, other := range actual {
				if reflect.DeepEqual(other, record) {
					found = true
					break
				}
			}
			assert.True(t, found)
		}
	})
}

func testDeleteNonExistantMessage(t *testing.T, s messaging.Store) {
	t.Run("testDeleteNonExistantMessage", func(t *testing.T) {
		ctx := context.Background()
		require.NoError(t, s.Delete(ctx, "account", uuid.New()))
	})
}

func makeUniqueRecordForAccount(account string) *messaging.Record {
	var message []byte
	for i := 0; i < 10; i++ {
		messagePart, _ := uuid.New().MarshalBinary()
		message = append(message, messagePart...)
	}
	return &messaging.Record{
		Account:   account,
		MessageID: uuid.New(),
		Message:   message,
	}
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *messaging.Record) {
	assert.Equal(t, obj1.Account, obj2.Account)
	assert.Equal(t, obj1.MessageID, obj2.MessageID)
	assert.Equal(t, obj1.Message, obj2.Message)
}
