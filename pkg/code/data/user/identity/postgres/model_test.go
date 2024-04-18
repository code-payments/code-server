package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
)

func TestModelConversion(t *testing.T) {
	phoneNumber := "+12223334444"
	record := &user_identity.Record{
		ID: user.NewID(),
		View: &user.View{
			PhoneNumber: &phoneNumber,
		},
		IsStaffUser: true,
		IsBanned:    true,
		CreatedAt:   time.Now(),
	}

	model, err := toModel(record)
	require.NoError(t, err)

	actual, err := fromModel(model)
	require.NoError(t, err)

	assertEqualUsers(t, record, actual)
}

func assertEqualUsers(t *testing.T, obj1, obj2 *user_identity.Record) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.ID, obj2.ID)
	assert.Equal(t, obj1.IsStaffUser, obj2.IsStaffUser)
	assert.Equal(t, obj1.IsBanned, obj2.IsBanned)
	assert.Equal(t, *obj1.View.PhoneNumber, *obj2.View.PhoneNumber)
	assert.Equal(t, obj1.CreatedAt.UTC(), obj2.CreatedAt.UTC())
}
