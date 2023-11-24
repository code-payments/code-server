package postgres

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"
)

func TestModelConversion(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	phoneNumber := "+12223334444"
	dataContainer := &user_storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: base58.Encode(pub),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}

	model, err := toModel(dataContainer)
	require.NoError(t, err)

	actual, err := fromModel(model)
	require.NoError(t, err)

	assertEqualDataContainers(t, dataContainer, actual)
}

func assertEqualDataContainers(t *testing.T, obj1, obj2 *user_storage.Record) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.ID, obj2.ID)
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.Equal(t, *obj1.IdentifyingFeatures.PhoneNumber, *obj2.IdentifyingFeatures.PhoneNumber)
	assert.Equal(t, obj1.CreatedAt.UTC(), obj2.CreatedAt.UTC())
}
