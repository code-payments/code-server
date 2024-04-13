package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
)

func RunTests(t *testing.T, s invite.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s invite.Store){
		testHappyPath,
		testInviteSameUserTwice,
		testExceedsInviteCount,
		testInvitedByNonInvitedUser,
		testFilterInvitedNumbers,
		testRevokedInvite,
		testGiveInvitesForDeposit,

		testInfluencerCodeClaim,
		testInfluencerCodeExpired,
		testInfluencerCodeRevoked,

		testInfluencerCodePutUser,
		testInfluencerCodeRevokedPutUser,
		testInfluencerCodeExpiredPutUser,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s invite.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		adminUser := &invite.User{
			PhoneNumber: "+11234567890",
			Invited:     time.Now(),
			InviteCount: 5,
		}
		require.NoError(t, s.PutUser(ctx, adminUser))

		actual, err := s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assertEqualUserEntries(t, adminUser, actual)

		invitedUser := &invite.User{
			PhoneNumber: "+12223334444",
			InvitedBy:   &adminUser.PhoneNumber,
			InviteCount: 3,
			Invited:     time.Now(),
		}

		_, err = s.GetUser(ctx, invitedUser.PhoneNumber)
		assert.Equal(t, invite.ErrUserNotFound, err)

		require.NoError(t, s.PutUser(ctx, invitedUser))

		actual, err = s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assert.EqualValues(t, 5, actual.InviteCount)
		assert.EqualValues(t, 1, actual.InvitesSent)

		actual, err = s.GetUser(ctx, invitedUser.PhoneNumber)
		require.NoError(t, err)
		assertEqualUserEntries(t, invitedUser, actual)
	})
}

func testInviteSameUserTwice(t *testing.T, s invite.Store) {
	t.Run("testInviteSameUserTwice", func(t *testing.T) {
		ctx := context.Background()

		adminUser := &invite.User{
			PhoneNumber: "+11234567890",
			Invited:     time.Now(),
			InviteCount: 5,
		}
		require.NoError(t, s.PutUser(ctx, adminUser))
		assert.Equal(t, invite.ErrAlreadyExists, s.PutUser(ctx, adminUser))

		invitedUser := &invite.User{
			PhoneNumber: "+12223334444",
			InvitedBy:   &adminUser.PhoneNumber,
			Invited:     time.Now(),
		}

		require.NoError(t, s.PutUser(ctx, invitedUser))
		assert.Equal(t, invite.ErrAlreadyExists, s.PutUser(ctx, invitedUser))

		actual, err := s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assert.EqualValues(t, 1, actual.InvitesSent)
	})
}

func testExceedsInviteCount(t *testing.T, s invite.Store) {
	t.Run("testExceedsInviteCount", func(t *testing.T) {
		ctx := context.Background()

		adminUser := &invite.User{
			PhoneNumber: "+11234567890",
			Invited:     time.Now(),
			InviteCount: 5,
		}
		require.NoError(t, s.PutUser(ctx, adminUser))

		for i := 0; i < 5; i++ {
			invitedUser := &invite.User{
				PhoneNumber: fmt.Sprintf("+1800555000%d", i),
				InvitedBy:   &adminUser.PhoneNumber,
				Invited:     time.Now(),
			}
			require.NoError(t, s.PutUser(ctx, invitedUser))
		}

		actual, err := s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		require.EqualValues(t, 5, actual.InviteCount)
		require.EqualValues(t, 5, actual.InvitesSent)

		invitedUser := &invite.User{
			PhoneNumber: "+12223334444",
			InvitedBy:   &adminUser.PhoneNumber,
			Invited:     time.Now(),
		}
		assert.Equal(t, invite.ErrInviteCountExceeded, s.PutUser(ctx, invitedUser))

		_, err = s.GetUser(ctx, invitedUser.PhoneNumber)
		assert.Equal(t, invite.ErrUserNotFound, err)

		actual, err = s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assert.EqualValues(t, 5, actual.InviteCount)
		assert.EqualValues(t, 5, actual.InvitesSent)
	})
}

func testInvitedByNonInvitedUser(t *testing.T, s invite.Store) {
	t.Run("testInvitedByNonInvitedUser", func(t *testing.T) {
		ctx := context.Background()

		invitedBy := "+11234567890"
		user := &invite.User{
			PhoneNumber: "+12223334444",
			InvitedBy:   &invitedBy,
			Invited:     time.Now(),
			InviteCount: 5,
		}
		assert.Equal(t, invite.ErrInviteCountExceeded, s.PutUser(ctx, user))
	})
}

func testFilterInvitedNumbers(t *testing.T, s invite.Store) {
	t.Run("testFilterInvitedNumbers", func(t *testing.T) {
		ctx := context.Background()

		adminUser := &invite.User{
			PhoneNumber: "+11234567890",
			Invited:     time.Now(),
			InviteCount: 100,
		}
		require.NoError(t, s.PutUser(ctx, adminUser))

		filtered, err := s.FilterInvitedNumbers(ctx, []string{})
		require.NoError(t, err)
		assert.Empty(t, filtered)

		var phoneNumbers []string
		for i := 0; i < 10; i++ {
			phoneNumbers = append(phoneNumbers, fmt.Sprintf("+1800555000%d", i))
		}

		filtered, err = s.FilterInvitedNumbers(ctx, phoneNumbers)
		require.NoError(t, err)
		assert.Empty(t, filtered)

		var invitedUsers []*invite.User
		for _, phoneNumber := range phoneNumbers[:len(phoneNumbers)/2] {
			invitedUser := &invite.User{
				PhoneNumber: phoneNumber,
				InvitedBy:   &adminUser.PhoneNumber,
				Invited:     time.Now(),
			}
			require.NoError(t, s.PutUser(ctx, invitedUser))

			invitedUsers = append(invitedUsers, invitedUser)
		}

		filtered, err = s.FilterInvitedNumbers(ctx, phoneNumbers)
		require.NoError(t, err)

		require.Len(t, filtered, len(phoneNumbers)/2)
		for i, actual := range filtered {
			assertEqualUserEntries(t, invitedUsers[i], actual)
		}
	})
}

func testRevokedInvite(t *testing.T, s invite.Store) {
	t.Run("testRevokedInvite", func(t *testing.T) {
		ctx := context.Background()

		adminUser := &invite.User{
			PhoneNumber: "+11234567890",
			Invited:     time.Now(),
			InviteCount: 100,
			IsRevoked:   true,
		}
		require.NoError(t, s.PutUser(ctx, adminUser))

		actual, err := s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assertEqualUserEntries(t, adminUser, actual)

		invitedUser := &invite.User{
			PhoneNumber: "+12223334444",
			InvitedBy:   &adminUser.PhoneNumber,
			Invited:     time.Now(),
		}
		err = s.PutUser(ctx, invitedUser)
		assert.Equal(t, invite.ErrInviteCountExceeded, err)

		_, err = s.GetUser(ctx, invitedUser.PhoneNumber)
		assert.Equal(t, invite.ErrUserNotFound, err)

		actual, err = s.GetUser(ctx, adminUser.PhoneNumber)
		require.NoError(t, err)
		assertEqualUserEntries(t, adminUser, actual)
	})
}

func testGiveInvitesForDeposit(t *testing.T, s invite.Store) {
	t.Run("testGiveInvitesForDeposit", func(t *testing.T) {
		ctx := context.Background()

		for i, alreadyReceived := range []bool{true, false} {
			inviteUser := &invite.User{
				PhoneNumber:            fmt.Sprintf("+1800555000%d", i),
				Invited:                time.Now(),
				InviteCount:            100,
				DepositInvitesReceived: alreadyReceived,
			}
			require.NoError(t, s.PutUser(ctx, inviteUser))

			actual, err := s.GetUser(ctx, inviteUser.PhoneNumber)
			require.NoError(t, err)
			assertEqualUserEntries(t, inviteUser, actual)

			for i := 0; i < 5; i++ {
				require.NoError(t, s.GiveInvitesForDeposit(ctx, inviteUser.PhoneNumber, 5))

				actual, err = s.GetUser(ctx, inviteUser.PhoneNumber)
				require.NoError(t, err)
				assert.True(t, actual.DepositInvitesReceived)
				if alreadyReceived {
					assert.EqualValues(t, 100, actual.InviteCount)
				} else {
					assert.EqualValues(t, 105, actual.InviteCount)
				}
			}
		}
	})
}

func assertEqualUserEntries(t *testing.T, obj1, obj2 *invite.User) {
	require.NoError(t, obj1.Validate())
	require.NoError(t, obj2.Validate())

	assert.Equal(t, obj1.PhoneNumber, obj2.PhoneNumber)
	assert.Equal(t, obj1.Invited.Unix(), obj2.Invited.Unix())
	assert.Equal(t, obj1.InviteCount, obj2.InviteCount)
	assert.Equal(t, obj1.InvitesSent, obj2.InvitesSent)
	assert.Equal(t, obj1.DepositInvitesReceived, obj2.DepositInvitesReceived)
	assert.Equal(t, obj1.IsRevoked, obj2.IsRevoked)

	if obj1.InvitedBy == nil {
		assert.Nil(t, obj2.InvitedBy)
	} else {
		require.NotNil(t, obj1.InvitedBy)
		assert.Equal(t, obj1.InvitedBy, obj2.InvitedBy)
	}
}

func testInfluencerCodeClaim(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodeClaim", func(t *testing.T) {
		ctx := context.Background()

		inviteCode := "mrbeast"

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        inviteCode,
			InviteCount: 10,
			InvitesSent: 2,
			IsRevoked:   false,
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		// Get the influencer code from the store
		actual, err := s.GetInfluencerCode(ctx, influencerCode.Code)
		require.NoError(t, err)

		// Assert the influencer code is the same
		assert.EqualValues(t, influencerCode.Code, actual.Code)
		assert.EqualValues(t, influencerCode.InviteCount, actual.InviteCount)
		assert.EqualValues(t, influencerCode.InvitesSent, actual.InvitesSent)
		assert.EqualValues(t, influencerCode.IsRevoked, actual.IsRevoked)
		assert.EqualValues(t, influencerCode.ExpiresAt.Unix(), actual.ExpiresAt.Unix())

		// Test multiple claims
		for i := uint32(influencerCode.InvitesSent + 1); i <= influencerCode.InviteCount; i++ {
			// Claim the influencer code
			err = s.ClaimInfluencerCode(ctx, inviteCode)
			assert.NoError(t, err)

			// Get the influencer code from the store
			actual, err = s.GetInfluencerCode(ctx, influencerCode.Code)

			// Assert that no error occurred
			assert.NoError(t, err)

			// Assert that the influencer code was claimed
			assert.EqualValues(t, influencerCode.Code, actual.Code)
			assert.EqualValues(t, influencerCode.InviteCount, actual.InviteCount)
			assert.EqualValues(t, i, actual.InvitesSent)
			assert.EqualValues(t, influencerCode.IsRevoked, actual.IsRevoked)
			assert.EqualValues(t, influencerCode.ExpiresAt.Unix(), actual.ExpiresAt.Unix())
		}

		// Claim the influencer code
		err = s.ClaimInfluencerCode(ctx, inviteCode)

		// Assert that the code has been used up
		assert.Equal(t, invite.ErrInviteCountExceeded, err)
	})
}

func testInfluencerCodeRevoked(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodeRevoked", func(t *testing.T) {
		ctx := context.Background()

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        "cristiano",
			InviteCount: 100,
			InvitesSent: 42,
			IsRevoked:   true,
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		// Claim the influencer code
		err := s.ClaimInfluencerCode(ctx, "cristiano")

		// Assert that the code has been revoked
		assert.Equal(t, invite.ErrInfluencerCodeRevoked, err)
	})
}

func testInfluencerCodeExpired(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodeExpired", func(t *testing.T) {
		ctx := context.Background()

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        "leomessi",
			InviteCount: 100,
			InvitesSent: 42,
			IsRevoked:   false,
			ExpiresAt:   time.Now().Add(-time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		// Claim the influencer code
		err := s.ClaimInfluencerCode(ctx, "leomessi")

		// Assert that the code has expired
		assert.Equal(t, invite.ErrInfluencerCodeExpired, err)
	})
}

func testInfluencerCodePutUser(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodePutUser", func(t *testing.T) {
		ctx := context.Background()

		invitedBy := "anatoly"

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        invitedBy,
			InviteCount: 10,
			InvitesSent: 2,
			IsRevoked:   false,
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		// Get the influencer code from the store
		actual, err := s.GetInfluencerCode(ctx, influencerCode.Code)
		require.NoError(t, err)

		// Assert the influencer code is the same
		assert.EqualValues(t, influencerCode.Code, actual.Code)
		assert.EqualValues(t, influencerCode.InviteCount, actual.InviteCount)
		assert.EqualValues(t, influencerCode.InvitesSent, actual.InvitesSent)
		assert.EqualValues(t, influencerCode.IsRevoked, actual.IsRevoked)
		assert.EqualValues(t, influencerCode.ExpiresAt.Unix(), actual.ExpiresAt.Unix())

		// Test multiple claims
		for i := uint32(influencerCode.InvitesSent + 1); i <= influencerCode.InviteCount; i++ {
			// Put a user to claim the code
			phoneNumber := fmt.Sprintf("+1800555000%d", i)
			invitedUser := &invite.User{
				PhoneNumber: phoneNumber,
				InvitedBy:   &invitedBy,
				Invited:     time.Now(),
			}
			require.NoError(t, s.PutUser(ctx, invitedUser))

			// Get the influencer code from the store
			actual, err = s.GetInfluencerCode(ctx, influencerCode.Code)

			// Assert that no error occurred
			assert.NoError(t, err)

			// Assert that the influencer code was claimed
			assert.EqualValues(t, influencerCode.Code, actual.Code)
			assert.EqualValues(t, influencerCode.InviteCount, actual.InviteCount)
			assert.EqualValues(t, i, actual.InvitesSent)
			assert.EqualValues(t, influencerCode.IsRevoked, actual.IsRevoked)
			assert.EqualValues(t, influencerCode.ExpiresAt.Unix(), actual.ExpiresAt.Unix())
		}

		phoneNumber := "+18005559001"
		invitedUser := &invite.User{
			PhoneNumber: phoneNumber,
			InvitedBy:   &invitedBy,
			Invited:     time.Now(),
		}

		// Assert that the code has been used up
		assert.Equal(t, invite.ErrInviteCountExceeded, s.PutUser(ctx, invitedUser))
	})
}

func testInfluencerCodeRevokedPutUser(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodeRevokedPutUser", func(t *testing.T) {
		ctx := context.Background()

		inviteCode := "vitalik"

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        inviteCode,
			InviteCount: 100,
			InvitesSent: 42,
			IsRevoked:   true,
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		phoneNumber := "+18005559001"
		invitedUser := &invite.User{
			PhoneNumber: phoneNumber,
			InvitedBy:   &inviteCode,
			Invited:     time.Now(),
		}

		// TODO: this should be ErrInviteCodeRevoked but following what the old code does...

		// Assert that the code has been revoked
		assert.Equal(t, invite.ErrInviteCountExceeded, s.PutUser(ctx, invitedUser))
	})
}

func testInfluencerCodeExpiredPutUser(t *testing.T, s invite.Store) {
	t.Run("testInfluencerCodeExpiredPutUser", func(t *testing.T) {
		ctx := context.Background()

		inviteCode := "lamport"

		// Create a influencer code
		influencerCode := &invite.InfluencerCode{
			Code:        inviteCode,
			InviteCount: 100,
			InvitesSent: 42,
			IsRevoked:   false,
			ExpiresAt:   time.Now().Add(-time.Hour),
		}

		// Add the influencer code to the store
		require.NoError(t, s.PutInfluencerCode(ctx, influencerCode))

		phoneNumber := "+18005559001"
		invitedUser := &invite.User{
			PhoneNumber: phoneNumber,
			InvitedBy:   &inviteCode,
			Invited:     time.Now(),
		}

		// TODO: this should be ErrInviteCodeExpired but following what the old code does...

		// Assert that the code has been revoked
		assert.Equal(t, invite.ErrInviteCountExceeded, s.PutUser(ctx, invitedUser))
	})
}
