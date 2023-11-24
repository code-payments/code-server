package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
)

func TestUserModelConversion(t *testing.T) {
	adminUser := &invite.User{
		PhoneNumber: "+11234567890",
		InvitedBy:   nil,
		Invited:     time.Now(),
		InviteCount: 5,
		InvitesSent: 2,
		IsRevoked:   false,
	}

	model, err := toUserModel(adminUser)
	require.NoError(t, err)

	actual := fromUserModel(model)
	assert.EqualValues(t, adminUser, actual)

	invitedUser := &invite.User{
		PhoneNumber: "+11234567890",
		InvitedBy:   &adminUser.PhoneNumber,
		Invited:     time.Now(),
		InviteCount: 5,
		InvitesSent: 2,
		IsRevoked:   true,
	}

	model, err = toUserModel(invitedUser)
	require.NoError(t, err)

	actual = fromUserModel(model)
	assert.EqualValues(t, invitedUser, actual)
}

func TestInfluencerCodeModelConversion(t *testing.T) {
	code := &invite.InfluencerCode{
		Code:        "test-code",
		InviteCount: 5,
		InvitesSent: 2,
		IsRevoked:   false,
		ExpiresAt:   time.Now(),
	}

	model, err := toInfluencerCodeModel(code)
	require.NoError(t, err)

	actual := fromInfluencerCodeModel(model)
	assert.EqualValues(t, code, actual)
}
