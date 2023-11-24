package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
)

func RunTests(t *testing.T, s user_identity.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s user_identity.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s user_identity.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		phoneNumber := "+12223334444"

		record := &user_identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			IsStaffUser: true,
			IsBanned:    true,
			CreatedAt:   time.Now(),
		}

		_, err := s.GetByID(ctx, record.ID)
		assert.Equal(t, user_identity.ErrNotFound, err)

		_, err = s.GetByView(ctx, record.View)
		assert.Equal(t, user_identity.ErrNotFound, err)

		require.NoError(t, s.Put(ctx, record))

		actual, err := s.GetByID(ctx, record.ID)
		require.NoError(t, err)
		assertEqualUsers(t, record, actual)

		actual, err = s.GetByView(ctx, record.View)
		require.NoError(t, err)
		assertEqualUsers(t, record, actual)

		err = s.Put(ctx, record)
		assert.Equal(t, user_identity.ErrAlreadyExists, err)
	})
}

func assertEqualUsers(t *testing.T, obj1, obj2 *user_identity.Record) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.ID, obj2.ID)
	assert.Equal(t, obj1.IsStaffUser, obj2.IsStaffUser)
	assert.Equal(t, obj1.IsBanned, obj2.IsBanned)
	assert.Equal(t, *obj1.View.PhoneNumber, *obj2.View.PhoneNumber)
	assert.Equal(t, obj1.CreatedAt.UTC().Truncate(time.Second), obj2.CreatedAt.UTC().Truncate(time.Second))
}
