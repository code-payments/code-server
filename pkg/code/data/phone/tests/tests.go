package tests

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	phoneutil "github.com/code-payments/code-server/pkg/phone"
	"github.com/code-payments/code-server/pkg/code/data/phone"
)

func RunTests(t *testing.T, s phone.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s phone.Store){
		testVerificationHappyPath,
		testGetVerification,
		testGetLatestVerificationForAccount,
		testGetLatestVerificationForNumber,
		testUpdateVerification,
		testGetAllVerificationsForNumber,
		testLinkingTokenHappyPath,
		testFilterVerifiedNumbers,
		testSettingsHappyPath,
		testEventHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testVerificationHappyPath(t *testing.T, s phone.Store) {
	t.Run("testVerificationHappyPath", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		verification := &phone.Verification{
			PhoneNumber:    "+11234567890",
			OwnerAccount:   base58.Encode(pub),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		}

		_, err = s.GetVerification(ctx, verification.OwnerAccount, verification.PhoneNumber)
		assert.Equal(t, phone.ErrVerificationNotFound, err)

		_, err = s.GetLatestVerificationForAccount(ctx, verification.OwnerAccount)
		assert.Equal(t, phone.ErrVerificationNotFound, err)

		_, err = s.GetLatestVerificationForNumber(ctx, verification.OwnerAccount)
		assert.Equal(t, phone.ErrVerificationNotFound, err)

		_, err = s.GetAllVerificationsForNumber(ctx, verification.PhoneNumber)
		assert.Equal(t, phone.ErrVerificationNotFound, err)

		require.NoError(t, s.SaveVerification(ctx, verification))

		actual, err := s.GetVerification(ctx, verification.OwnerAccount, verification.PhoneNumber)
		require.NoError(t, err)
		assertEqualVerifications(t, verification, actual)

		actual, err = s.GetLatestVerificationForAccount(ctx, verification.OwnerAccount)
		require.NoError(t, err)
		assertEqualVerifications(t, verification, actual)

		actual, err = s.GetLatestVerificationForNumber(ctx, verification.PhoneNumber)
		require.NoError(t, err)
		assertEqualVerifications(t, verification, actual)

		all, err := s.GetAllVerificationsForNumber(ctx, verification.PhoneNumber)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assertEqualVerifications(t, verification, all[0])
	})
}

func testGetVerification(t *testing.T, s phone.Store) {
	t.Run("testGetVerification", func(t *testing.T) {
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			pub, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)

			verification := &phone.Verification{
				PhoneNumber:    fmt.Sprintf("+1800555000%d", i),
				OwnerAccount:   base58.Encode(pub),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}

			_, err = s.GetVerification(ctx, verification.OwnerAccount, verification.PhoneNumber)
			assert.Equal(t, phone.ErrVerificationNotFound, err)

			require.NoError(t, s.SaveVerification(ctx, verification))

			actual, err := s.GetVerification(ctx, verification.OwnerAccount, verification.PhoneNumber)
			require.NoError(t, err)
			assertEqualVerifications(t, verification, actual)
		}
	})
}

func testGetLatestVerificationForAccount(t *testing.T, s phone.Store) {
	t.Run("testGetLatestVerificationForAccount", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		ownerAccount := base58.Encode(pub)

		var verifications []*phone.Verification
		for i := 0; i < 3; i++ {
			verification := &phone.Verification{
				PhoneNumber:    fmt.Sprintf("+1800555000%d", i),
				OwnerAccount:   ownerAccount,
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.NoError(t, s.SaveVerification(ctx, verification))

			verifications = append(verifications, verification)
		}

		actual, err := s.GetLatestVerificationForAccount(ctx, ownerAccount)
		require.NoError(t, err)
		assertEqualVerifications(t, verifications[len(verifications)-1], actual)
	})
}

func testGetLatestVerificationForNumber(t *testing.T, s phone.Store) {
	t.Run("testGetLatestVerificationForNumber", func(t *testing.T) {
		ctx := context.Background()

		var verifications []*phone.Verification
		for i := 0; i < 3; i++ {
			pub, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)

			verification := &phone.Verification{
				PhoneNumber:    "+11234567890",
				OwnerAccount:   base58.Encode(pub),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.NoError(t, s.SaveVerification(ctx, verification))

			verifications = append(verifications, verification)
		}

		actual, err := s.GetLatestVerificationForNumber(ctx, verifications[0].PhoneNumber)
		require.NoError(t, err)
		assertEqualVerifications(t, verifications[len(verifications)-1], actual)
	})
}

func testUpdateVerification(t *testing.T, s phone.Store) {
	t.Run("testUpdateVerification", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		verification := &phone.Verification{
			PhoneNumber:    "+11234567890",
			OwnerAccount:   base58.Encode(pub),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		}

		require.NoError(t, s.SaveVerification(ctx, verification))

		assert.Equal(t, phone.ErrInvalidVerification, s.SaveVerification(ctx, verification))

		verification.CreatedAt = verification.CreatedAt.Add(1 * time.Hour)
		verification.LastVerifiedAt = verification.LastVerifiedAt.Add(1 * time.Hour)
		require.NoError(t, s.SaveVerification(ctx, verification))

		actual, err := s.GetLatestVerificationForAccount(ctx, verification.OwnerAccount)
		require.NoError(t, err)
		verification.CreatedAt = verification.CreatedAt.Add(-1 * time.Hour)
		assertEqualVerifications(t, verification, actual)

		verification.PhoneNumber = "+12223334444"
		verification.LastVerifiedAt = verification.LastVerifiedAt.Add(1 * time.Hour)
		require.NoError(t, s.SaveVerification(ctx, verification))

		actual, err = s.GetLatestVerificationForAccount(ctx, verification.OwnerAccount)
		require.NoError(t, err)
		assertEqualVerifications(t, verification, actual)
	})
}

func testGetAllVerificationsForNumber(t *testing.T, s phone.Store) {
	t.Run("testGetAllVerificationsForNumber", func(t *testing.T) {
		ctx := context.Background()

		var verifications []*phone.Verification
		for i := 0; i < 3; i++ {
			pub, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)

			verification := &phone.Verification{
				PhoneNumber:    "+11234567890",
				OwnerAccount:   base58.Encode(pub),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.NoError(t, s.SaveVerification(ctx, verification))

			verifications = append(verifications, verification)
		}

		all, err := s.GetAllVerificationsForNumber(ctx, verifications[0].PhoneNumber)
		require.NoError(t, err)
		require.Len(t, all, len(verifications))
		for i, actual := range all {
			assertEqualVerifications(t, verifications[len(verifications)-i-1], actual)
		}
	})
}

func testLinkingTokenHappyPath(t *testing.T, s phone.Store) {
	t.Run("testLinkingTokenHappyPath", func(t *testing.T) {
		ctx := context.Background()

		token := &phone.LinkingToken{
			PhoneNumber:   "+11234567890",
			Code:          "123456",
			ExpiresAt:     time.Now().Add(1 * time.Hour),
			MaxCheckCount: 1,
		}

		err := s.UseLinkingToken(ctx, token.PhoneNumber, token.Code)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)

		require.NoError(t, s.SaveLinkingToken(ctx, token))

		require.NoError(t, s.UseLinkingToken(ctx, token.PhoneNumber, token.Code))
		err = s.UseLinkingToken(ctx, token.PhoneNumber, token.Code)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)

		originalCode := token.Code
		require.NoError(t, s.SaveLinkingToken(ctx, token))
		token.Code = "7890"
		token.MaxCheckCount = 2
		require.NoError(t, s.SaveLinkingToken(ctx, token))

		err = s.UseLinkingToken(ctx, token.PhoneNumber, originalCode)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)

		require.NoError(t, s.UseLinkingToken(ctx, token.PhoneNumber, token.Code))
		err = s.UseLinkingToken(ctx, token.PhoneNumber, token.Code)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)

		require.NoError(t, s.SaveLinkingToken(ctx, token))
		for i := 0; i < int(token.MaxCheckCount); i++ {
			err = s.UseLinkingToken(ctx, token.PhoneNumber, originalCode)
			assert.Equal(t, phone.ErrLinkingTokenNotFound, err)
		}

		err = s.UseLinkingToken(ctx, token.PhoneNumber, token.Code)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)
	})
}

func testFilterVerifiedNumbers(t *testing.T, s phone.Store) {
	t.Run("testFilterVerifiedNumbers", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		ownerAccount := base58.Encode(pub)

		var phoneNumbers []string
		for i := 0; i < 10; i++ {
			phoneNumbers = append(phoneNumbers, fmt.Sprintf("+1800555000%d", i))
		}

		filtered, err := s.FilterVerifiedNumbers(ctx, phoneNumbers)
		require.NoError(t, err)
		assert.Empty(t, filtered)

		for i, phoneNumber := range phoneNumbers[:len(phoneNumbers)/2] {
			verification := &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount,
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.NoError(t, s.SaveVerification(ctx, verification))
		}

		filtered, err = s.FilterVerifiedNumbers(ctx, phoneNumbers)
		require.NoError(t, err)
		assert.Equal(t, phoneNumbers[:len(phoneNumbers)/2], filtered)
	})
}

func testSettingsHappyPath(t *testing.T, s phone.Store) {
	t.Run("testSettingsHappyPath", func(t *testing.T) {
		ctx := context.Background()

		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		ownerAccount := base58.Encode(pub)

		phoneNumber := "+11234567890"

		actual, err := s.GetSettings(ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, actual.PhoneNumber)
		assert.Empty(t, actual.ByOwnerAccount)

		now := time.Now()
		expectedSettings := &phone.OwnerAccountSetting{
			OwnerAccount:  ownerAccount,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}

		require.NoError(t, s.SaveOwnerAccountSetting(ctx, phoneNumber, expectedSettings))

		actual, err = s.GetSettings(ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, actual.PhoneNumber)
		assert.Len(t, actual.ByOwnerAccount, 1)

		actualForOwnerAccount, ok := actual.ByOwnerAccount[ownerAccount]
		require.True(t, ok)
		assertEqualOwnerAccountSettings(t, expectedSettings, actualForOwnerAccount)

		now = time.Now()
		isUnlinked := true
		expectedSettings.IsUnlinked = &isUnlinked
		expectedSettings.CreatedAt = now
		expectedSettings.LastUpdatedAt = now
		require.NoError(t, s.SaveOwnerAccountSetting(ctx, phoneNumber, expectedSettings))

		actual, err = s.GetSettings(ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, actual.PhoneNumber)
		assert.Len(t, actual.ByOwnerAccount, 1)

		actualForOwnerAccount, ok = actual.ByOwnerAccount[ownerAccount]
		require.True(t, ok)
		assert.Equal(t, ownerAccount, actualForOwnerAccount.OwnerAccount)
		require.NotNil(t, actualForOwnerAccount.IsUnlinked)
		assert.True(t, *actualForOwnerAccount.IsUnlinked)
		assert.True(t, actualForOwnerAccount.LastUpdatedAt.After(actualForOwnerAccount.CreatedAt))
		assert.Equal(t, expectedSettings.LastUpdatedAt.Unix(), actualForOwnerAccount.LastUpdatedAt.Unix())

		isUnlinked = false
		expectedSettings.LastUpdatedAt = time.Now()
		require.NoError(t, s.SaveOwnerAccountSetting(ctx, phoneNumber, expectedSettings))

		actual, err = s.GetSettings(ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, actual.PhoneNumber)
		assert.Len(t, actual.ByOwnerAccount, 1)

		actualForOwnerAccount, ok = actual.ByOwnerAccount[ownerAccount]
		require.True(t, ok)
		require.NotNil(t, actualForOwnerAccount.IsUnlinked)
		assert.False(t, *actualForOwnerAccount.IsUnlinked)

		expectedSettings.IsUnlinked = nil
		expectedSettings.LastUpdatedAt = time.Now()
		require.NoError(t, s.SaveOwnerAccountSetting(ctx, phoneNumber, expectedSettings))

		actual, err = s.GetSettings(ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, actual.PhoneNumber)
		assert.Len(t, actual.ByOwnerAccount, 1)

		actualForOwnerAccount, ok = actual.ByOwnerAccount[ownerAccount]
		require.True(t, ok)
		require.NotNil(t, actualForOwnerAccount.IsUnlinked)
		assert.False(t, *actualForOwnerAccount.IsUnlinked)
	})
}

func testEventHappyPath(t *testing.T, s phone.Store) {
	t.Run("testEventHappyPath", func(t *testing.T) {
		start := time.Now()

		ctx := context.Background()

		for i := 0; i < 5; i++ {
			expected := &phone.Event{
				Type:           phone.EventTypeVerificationCodeSent,
				VerificationId: "verification_id",
				PhoneNumber:    "+12223334444",
				PhoneMetadata: &phoneutil.Metadata{
					PhoneNumber: "+12223334444",
				},
				CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
			}

			count, err := s.CountUniqueVerificationIdsForNumberSinceTimestamp(ctx, expected.PhoneNumber, start)
			require.NoError(t, err)
			if i == 0 {
				assert.EqualValues(t, 0, count)
			} else {
				assert.EqualValues(t, 1, count)
			}

			count, err = s.CountEventsForVerificationByType(ctx, expected.VerificationId, expected.Type)
			require.NoError(t, err)
			assert.EqualValues(t, i, count)

			count, err = s.CountEventsForNumberByTypeSinceTimestamp(ctx, expected.PhoneNumber, expected.Type, start)
			require.NoError(t, err)
			assert.EqualValues(t, i, count)

			require.NoError(t, s.PutEvent(ctx, expected))

			actual, err := s.GetLatestEventForNumberByType(ctx, expected.PhoneNumber, expected.Type)
			require.NoError(t, err)
			assertEqualEvents(t, expected, actual)
		}

		for i := 0; i < 5; i++ {
			event := &phone.Event{
				Type:           phone.EventTypeVerificationCodeSent,
				VerificationId: fmt.Sprintf("verification%d", i),
				PhoneNumber:    "+12223334444",
				PhoneMetadata: &phoneutil.Metadata{
					PhoneNumber: "+12223334444",
				},
				CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
			}

			count, err := s.CountUniqueVerificationIdsForNumberSinceTimestamp(ctx, event.PhoneNumber, start)
			require.NoError(t, err)
			assert.EqualValues(t, i+1, count)

			require.NoError(t, s.PutEvent(ctx, event))
		}

		// Phone number doesn't have any events
		_, err := s.GetLatestEventForNumberByType(ctx, "+18005550000", phone.EventTypeVerificationCodeSent)
		assert.Equal(t, phone.ErrEventNotFound, err)

		// Phone number doesn't have any events of the provided type
		_, err = s.GetLatestEventForNumberByType(ctx, "+12223334444", phone.EventTypeCheckVerificationCode)
		assert.Equal(t, phone.ErrEventNotFound, err)

		// Verification doesn't exist
		count, err := s.CountEventsForVerificationByType(ctx, "unknown_verification", phone.EventTypeVerificationCodeSent)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Phone number doesn't have any event of the provided type
		count, err = s.CountEventsForNumberByTypeSinceTimestamp(ctx, "+18005550000", phone.EventTypeVerificationCodeSent, start)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Timestamp doesn't capture any verifications
		count, err = s.CountEventsForNumberByTypeSinceTimestamp(ctx, "+12223334444", phone.EventTypeVerificationCodeSent, time.Now().Add(time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Number doesn't have any verifications
		count, err = s.CountUniqueVerificationIdsForNumberSinceTimestamp(ctx, "+18005550000", start)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		// Timestamp doesn't capture any verifications
		count, err = s.CountUniqueVerificationIdsForNumberSinceTimestamp(ctx, "+12223334444", time.Now().Add(time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)
	})
}

func assertEqualVerifications(t *testing.T, obj1, obj2 *phone.Verification) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.PhoneNumber, obj2.PhoneNumber)
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
	assert.Equal(t, obj1.LastVerifiedAt.Unix(), obj2.LastVerifiedAt.Unix())
}

func assertEqualOwnerAccountSettings(t *testing.T, obj1, obj2 *phone.OwnerAccountSetting) {
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.EqualValues(t, obj1.IsUnlinked, obj2.IsUnlinked)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
	assert.Equal(t, obj1.LastUpdatedAt.Unix(), obj2.LastUpdatedAt.Unix())
}

func assertEqualEvents(t *testing.T, obj1, obj2 *phone.Event) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.Type, obj2.Type)
	assert.Equal(t, obj1.VerificationId, obj2.VerificationId)
	assert.Equal(t, obj1.PhoneNumber, obj2.PhoneNumber)
	assert.Equal(t, obj1.PhoneMetadata.PhoneNumber, obj2.PhoneMetadata.PhoneNumber)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}
