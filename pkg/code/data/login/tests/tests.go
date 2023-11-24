package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/login"
)

func RunTests(t *testing.T, s login.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s login.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s login.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllByInstallId(ctx, "app-install-1")
		assert.Equal(t, login.ErrLoginNotFound, err)

		_, err = s.GetLatestByOwner(ctx, "owner1")
		assert.Equal(t, login.ErrLoginNotFound, err)

		start := time.Now()
		expected := &login.MultiRecord{
			AppInstallId: "app-install-1",
			Owners:       []string{"owner1"},
		}
		require.NoError(t, s.Save(ctx, expected))
		assert.Equal(t, "app-install-1", expected.AppInstallId)
		assert.Equal(t, "owner1", expected.Owners[0])
		assert.True(t, expected.LastUpdatedAt.After(start))

		expected = &login.MultiRecord{
			AppInstallId: "app-install-2",
			Owners:       []string{"owner2"},
		}
		require.NoError(t, s.Save(ctx, expected))
		assert.Equal(t, "app-install-2", expected.AppInstallId)
		assert.Equal(t, "owner2", expected.Owners[0])
		assert.True(t, expected.LastUpdatedAt.After(start))

		multiActual, err := s.GetAllByInstallId(ctx, "app-install-1")
		require.NoError(t, err)
		assert.Equal(t, "app-install-1", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner1", multiActual.Owners[0])

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-2")
		require.NoError(t, err)
		assert.Equal(t, "app-install-2", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner2", multiActual.Owners[0])

		require.NoError(t, s.Save(ctx, &login.MultiRecord{
			AppInstallId: "app-install-3",
			Owners:       []string{"owner1"},
		}))

		_, err = s.GetAllByInstallId(ctx, "app-install-1")
		assert.Equal(t, login.ErrLoginNotFound, err)

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-2")
		require.NoError(t, err)
		assert.Equal(t, "app-install-2", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner2", multiActual.Owners[0])

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-3")
		require.NoError(t, err)
		assert.Equal(t, "app-install-3", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner1", multiActual.Owners[0])

		start = time.Now()
		expected = &login.MultiRecord{
			AppInstallId: "app-install-2",
			Owners:       []string{"owner3"},
		}
		require.NoError(t, s.Save(ctx, expected))
		assert.Equal(t, "app-install-2", expected.AppInstallId)
		assert.Equal(t, "owner3", expected.Owners[0])
		assert.True(t, expected.LastUpdatedAt.After(start))

		_, err = s.GetAllByInstallId(ctx, "app-install-1")
		assert.Equal(t, login.ErrLoginNotFound, err)

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-2")
		require.NoError(t, err)
		assert.Equal(t, "app-install-2", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner3", multiActual.Owners[0])

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-3")
		require.NoError(t, err)
		assert.Equal(t, "app-install-3", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner1", multiActual.Owners[0])

		start = time.Now()
		expected = &login.MultiRecord{
			AppInstallId: "app-install-2",
			Owners:       nil,
		}
		require.NoError(t, s.Save(ctx, expected))
		assert.Equal(t, "app-install-2", expected.AppInstallId)
		assert.Empty(t, expected.Owners)
		assert.True(t, expected.LastUpdatedAt.After(start))

		_, err = s.GetAllByInstallId(ctx, "app-install-1")
		assert.Equal(t, login.ErrLoginNotFound, err)

		_, err = s.GetAllByInstallId(ctx, "app-install-2")
		assert.Equal(t, login.ErrLoginNotFound, err)

		multiActual, err = s.GetAllByInstallId(ctx, "app-install-3")
		require.NoError(t, err)
		assert.Equal(t, "app-install-3", multiActual.AppInstallId)
		require.Len(t, multiActual.Owners, 1)
		assert.Equal(t, "owner1", multiActual.Owners[0])

		singleActual, err := s.GetLatestByOwner(ctx, "owner1")
		require.NoError(t, err)
		assert.Equal(t, "app-install-3", singleActual.AppInstallId)
		assert.Equal(t, "owner1", singleActual.Owner)

		_, err = s.GetLatestByOwner(ctx, "owner2")
		assert.Equal(t, login.ErrLoginNotFound, err)

		_, err = s.GetLatestByOwner(ctx, "owner3")
		assert.Equal(t, login.ErrLoginNotFound, err)
	})
}
