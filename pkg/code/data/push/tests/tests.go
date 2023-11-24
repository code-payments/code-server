package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

func RunTests(t *testing.T, s push.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s push.Store){
		testHappyPath,
		testMarkAsInvalid,
		testDelete,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s push.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		dataContainer := user.NewDataContainerID()

		_, err := s.GetAllValidByDataContainer(ctx, dataContainer)
		assert.Equal(t, push.ErrTokenNotFound, err)

		var expected []*push.Record
		for i := 0; i < 5; i++ {
			tokenType := push.TokenTypeFcmAndroid
			appInstallId := pointer.String("test_app_install")
			if i%2 == 0 {
				tokenType = push.TokenTypeFcmApns
				appInstallId = nil
			}

			record := &push.Record{
				DataContainerId: *dataContainer,

				PushToken: fmt.Sprintf("test_token_%d", i),
				TokenType: tokenType,
				IsValid:   true,

				AppInstallId: appInstallId,

				CreatedAt: time.Now(),
			}
			require.NoError(t, s.Put(ctx, record))

			expected = append(expected, record)
		}

		actual, err := s.GetAllValidByDataContainer(ctx, dataContainer)
		require.NoError(t, err)

		require.Len(t, actual, len(expected))

		for i := 0; i < len(expected); i++ {
			assert.EqualValues(t, i+1, actual[i].Id)
			assert.EqualValues(t, i+1, expected[i].Id)

			assert.Equal(t, expected[i].DataContainerId.String(), actual[i].DataContainerId.String())

			assert.Equal(t, expected[i].PushToken, actual[i].PushToken)
			assert.Equal(t, expected[i].TokenType, actual[i].TokenType)
			assert.True(t, actual[i].IsValid)

			assert.EqualValues(t, expected[i].AppInstallId, actual[i].AppInstallId)

			assert.Equal(t, expected[i].CreatedAt.Unix(), actual[i].CreatedAt.Unix())
		}

		for _, record := range expected {
			err := s.Put(ctx, record)
			assert.Equal(t, push.ErrTokenExists, err)
		}
	})
}

func testMarkAsInvalid(t *testing.T, s push.Store) {
	t.Run("testMarkAsInvalid", func(t *testing.T) {
		ctx := context.Background()

		var dataContainers []*user.DataContainerID
		for i := 0; i < 5; i++ {
			dataContainers = append(dataContainers, user.NewDataContainerID())
		}

		err := s.MarkAsInvalid(ctx, "invalid_token")
		require.NoError(t, err)

		for _, dataContainer := range dataContainers {
			for _, token := range []string{"valid_token", "invalid_token"} {
				record := &push.Record{
					DataContainerId: *dataContainer,

					PushToken: token,
					TokenType: push.TokenTypeFcmAndroid,
					IsValid:   true,

					CreatedAt: time.Now(),
				}
				require.NoError(t, s.Put(ctx, record))
			}
		}

		for _, dataContainer := range dataContainers {
			actual, err := s.GetAllValidByDataContainer(ctx, dataContainer)
			require.NoError(t, err)
			assert.Len(t, actual, 2)
		}

		err = s.MarkAsInvalid(ctx, "invalid_token")
		require.NoError(t, err)

		for _, dataContainer := range dataContainers {
			actual, err := s.GetAllValidByDataContainer(ctx, dataContainer)
			require.NoError(t, err)
			require.Len(t, actual, 1)
			assert.Equal(t, "valid_token", actual[0].PushToken)
		}
	})
}

func testDelete(t *testing.T, s push.Store) {
	t.Run("testDelete", func(t *testing.T) {
		ctx := context.Background()

		var dataContainers []*user.DataContainerID
		for i := 0; i < 5; i++ {
			dataContainers = append(dataContainers, user.NewDataContainerID())
		}

		err := s.Delete(ctx, "push_token_1")
		require.NoError(t, err)

		for _, dataContainer := range dataContainers {
			for _, token := range []string{"push_token_1", "push_token_2"} {
				record := &push.Record{
					DataContainerId: *dataContainer,

					PushToken: token,
					TokenType: push.TokenTypeFcmAndroid,
					IsValid:   true,

					CreatedAt: time.Now(),
				}
				require.NoError(t, s.Put(ctx, record))
			}
		}

		for _, dataContainer := range dataContainers {
			actual, err := s.GetAllValidByDataContainer(ctx, dataContainer)
			require.NoError(t, err)
			assert.Len(t, actual, 2)
		}

		err = s.Delete(ctx, "push_token_1")
		require.NoError(t, err)

		for _, dataContainer := range dataContainers {
			actual, err := s.GetAllValidByDataContainer(ctx, dataContainer)
			require.NoError(t, err)
			require.Len(t, actual, 1)
			assert.Equal(t, "push_token_2", actual[0].PushToken)
		}
	})
}
