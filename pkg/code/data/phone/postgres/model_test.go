package postgres

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	phoneutil "github.com/code-payments/code-server/pkg/phone"
	"github.com/code-payments/code-server/pkg/code/data/phone"
)

func TestVerificationModelConversion(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	verification := &phone.Verification{
		PhoneNumber:    "+11234567890",
		OwnerAccount:   base58.Encode(pub),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now().Add(1 * time.Hour),
	}

	model, err := toVerificationModel(verification)
	require.NoError(t, err)

	actual := fromVerificationModel(model)
	assert.EqualValues(t, verification, actual)
}

func TestLinkingTokenModelConversion(t *testing.T) {
	token := &phone.LinkingToken{
		PhoneNumber:       "+11234567890",
		Code:              "123456",
		CurrentCheckCount: 1,
		MaxCheckCount:     5,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
	}

	model, err := toLinkingTokenModel(token)
	require.NoError(t, err)

	actual := fromLinkingTokenModel(model)
	assert.EqualValues(t, token, actual)
}

func TestOwnerAccountSettingModelConversion(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	phoneNumber := "+11234567890"

	trueVal := true
	for _, isUnlinked := range []*bool{nil, &trueVal} {
		setting := &phone.OwnerAccountSetting{
			OwnerAccount:  base58.Encode(pub),
			CreatedAt:     time.Now(),
			IsUnlinked:    isUnlinked,
			LastUpdatedAt: time.Now().Add(1 * time.Hour),
		}

		model, err := toOwnerAccountSettingModel(phoneNumber, setting)
		require.NoError(t, err)

		assert.Equal(t, phoneNumber, model.PhoneNumber)

		actual := fromOwnerAccountSettingModel(model)
		assert.EqualValues(t, setting, actual)
	}
}

func TestEventModelConversion(t *testing.T) {
	phoneType := phoneutil.TypeMobile
	mcc := 302
	mnc := 720
	event := &phone.Event{
		Type:           phone.EventTypeVerificationCodeSent,
		VerificationId: "verification_id",
		PhoneNumber:    "+12223334444",
		PhoneMetadata: &phoneutil.Metadata{
			PhoneNumber:       "+12223334444",
			Type:              &phoneType,
			MobileCountryCode: &mcc,
			MobileNetworkCode: &mnc,
		},
		CreatedAt: time.Now(),
	}

	model, err := toEventModel(event)
	require.NoError(t, err)

	actual := fromEventModel(model)
	assert.EqualValues(t, event, actual)
}
