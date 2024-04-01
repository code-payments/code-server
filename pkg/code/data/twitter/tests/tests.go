package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
)

func RunTests(t *testing.T, s twitter.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s twitter.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s twitter.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		username := "jeffyanta"

		_, err := s.Get(ctx, username)
		assert.Equal(t, twitter.ErrUserNotFound, err)

		expected := &twitter.Record{
			Username:      username,
			Name:          "Jeff",
			ProfilePicUrl: "https://pbs.twimg.com/profile_images/1728595562285441024/GM-aLyh__normal.jpg",
			VerifiedType:  userpb.GetTwitterUserResponse_BLUE,
			FollowerCount: 200,
			TipAddress:    "tip_address_1",
		}
		cloned := expected.Clone()

		start := time.Now()
		require.NoError(t, s.Save(ctx, expected))
		assert.EqualValues(t, 1, expected.Id)
		assert.True(t, expected.CreatedAt.After(start))
		assert.True(t, expected.LastUpdatedAt.After(start))

		actual, err := s.Get(ctx, username)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		expected.Name = "Jeff Yanta"
		expected.ProfilePicUrl = "https://pbs.twimg.com/profile_images/1728595562285441024/GM-aLyh__highres.jpg"
		expected.VerifiedType = userpb.GetTwitterUserResponse_NONE
		expected.FollowerCount = 1000
		expected.TipAddress = "tip_address_2"
		cloned = expected.Clone()
		require.NoError(t, s.Save(ctx, expected))
		assert.True(t, expected.LastUpdatedAt.After(expected.CreatedAt))

		actual, err = s.Get(ctx, username)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *twitter.Record) {
	assert.Equal(t, obj1.Username, obj2.Username)
	assert.Equal(t, obj1.Name, obj2.Name)
	assert.Equal(t, obj1.ProfilePicUrl, obj2.ProfilePicUrl)
	assert.Equal(t, obj1.VerifiedType, obj2.VerifiedType)
	assert.Equal(t, obj1.FollowerCount, obj2.FollowerCount)
	assert.Equal(t, obj1.TipAddress, obj2.TipAddress)
}
