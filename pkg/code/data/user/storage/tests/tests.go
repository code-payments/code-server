package tests

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"
)

func RunTests(t *testing.T, s user_storage.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s user_storage.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s user_storage.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		phoneNumber := "+12223334444"

		record := &user_storage.Record{
			ID:           user.NewDataContainerID(),
			OwnerAccount: base58.Encode(pub),
			IdentifyingFeatures: &user.IdentifyingFeatures{
				PhoneNumber: &phoneNumber,
			},
			CreatedAt: time.Now(),
		}

		_, err = s.GetByID(ctx, record.ID)
		assert.Equal(t, user_storage.ErrNotFound, err)

		_, err = s.GetByFeatures(ctx, record.OwnerAccount, record.IdentifyingFeatures)
		assert.Equal(t, user_storage.ErrNotFound, err)

		require.NoError(t, s.Put(ctx, record))

		actual, err := s.GetByID(ctx, record.ID)
		require.NoError(t, err)
		assertEqualDataContainers(t, record, actual)

		actual, err = s.GetByFeatures(ctx, record.OwnerAccount, record.IdentifyingFeatures)
		require.NoError(t, err)
		assertEqualDataContainers(t, record, actual)

		err = s.Put(ctx, record)
		assert.Equal(t, user_storage.ErrAlreadyExists, err)
	})
}

func assertEqualDataContainers(t *testing.T, obj1, obj2 *user_storage.Record) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.ID, obj2.ID)
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.Equal(t, *obj1.IdentifyingFeatures.PhoneNumber, *obj2.IdentifyingFeatures.PhoneNumber)
	assert.Equal(t, obj1.CreatedAt.UTC().Truncate(time.Second), obj2.CreatedAt.UTC().Truncate(time.Second))
}
