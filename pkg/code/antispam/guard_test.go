package antispam

/*
import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/currency"
	memory_device_verifier "github.com/code-payments/code-server/pkg/device/memory"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx   context.Context
	guard *Guard
	data  code_data.Provider
}

func setup(t *testing.T) (env testEnv) {
	env.ctx = context.Background()
	env.data = code_data.NewTestDataProvider()
	env.guard = NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,

		// Intent limits
		WithDailyPaymentLimit(5),
		WithPaymentRateLimit(time.Second),
		WithMaxNewRelationshipsPerDay(5),
	)

	return env
}

func TestAllowSendPayment_NotPhoneVerified(t *testing.T) {
	env := setup(t)

	// Account isn't phone verified, so it cannot be used for payments
	for _, isPublic := range []bool{true, false} {
		for i := 0; i < 5; i++ {
			allow, err := env.guard.AllowSendPayment(env.ctx, testutil.NewRandomAccount(t), isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.False(t, allow)
		}
	}
}

func TestAllowSendPayment_TimeBetweenIntents(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		phoneNumber := "+12223334444"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}

		// First payment should always succeed
		allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount1, isPublic, testutil.NewRandomAccount(t))
		require.NoError(t, err)
		assert.True(t, allow)

		// Subsequent payments should fail hitting the time-based rate limit
		// regardless of owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			allow, err = env.guard.AllowSendPayment(env.ctx, ownerAccount, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.False(t, allow)
		}

		// After waiting the timeout, the payments should be allowed regardless of
		// owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			// todo: need a better way to test with time than waiting
			time.Sleep(time.Second)

			allow, err = env.guard.AllowSendPayment(env.ctx, ownerAccount, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.True(t, allow)
		}
	}
}

func TestAllowSendPayment_TimeBasedLimit(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		phoneNumber := "+12223334444"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}

		// First payment should always succeed
		allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount1, isPublic, testutil.NewRandomAccount(t))
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the daily limit of payments
		for _, state := range []intent.State{
			intent.StateUnknown,
			intent.StatePending,
			intent.StateFailed,
			intent.StateConfirmed,
		} {
			for i := 0; i < 2; i++ {
				simulateSentPayment(t, env, ownerAccount1, isPublic, state)
			}
		}

		// Payments are denied after breaching the daily count limit regardless of
		// owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			// todo: need a better way to test with time than waiting
			time.Sleep(time.Second)

			allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.False(t, allow)
		}
	}
}

func TestAllowSendPayment_PublicAndPrivateSeparated(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		phoneNumber := "+12223334444"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}

		// First payment should always succeed
		allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount1, isPublic, testutil.NewRandomAccount(t))
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the daily limit of payments of the other type
		for _, state := range []intent.State{
			intent.StateUnknown,
			intent.StatePending,
			intent.StateFailed,
			intent.StateConfirmed,
		} {
			for i := 0; i < 2; i++ {
				simulateSentPayment(t, env, ownerAccount1, !isPublic, state)
			}
		}

		// Payments are still allowed after breaching the daily count limit of
		// the other kind
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			// todo: need a better way to test with time than waiting
			time.Sleep(time.Second)

			allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.True(t, allow)
		}
	}
}

func TestAllowSendPayment_StaffUser(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		for i, isStaffUser := range []bool{true, false} {
			phoneNumber := fmt.Sprintf("+1800555000%d", i)

			ownerAccount1 := testutil.NewRandomAccount(t)
			ownerAccount2 := testutil.NewRandomAccount(t)

			require.NoError(t, env.data.PutUser(env.ctx, &identity.Record{
				ID: user.NewUserID(),
				View: &user.View{
					PhoneNumber: &phoneNumber,
				},
				IsStaffUser: isStaffUser,
				CreatedAt:   time.Now(),
			}))

			for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
				verification := &phone.Verification{
					PhoneNumber:    phoneNumber,
					OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
					CreatedAt:      time.Now(),
					LastVerifiedAt: time.Now(),
				}
				require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
			}

			// First payment should always be successful, regardless of user status
			allow, err := env.guard.AllowSendPayment(env.ctx, ownerAccount1, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.True(t, allow)

			// Consume the remaining daily limits for payments.
			for i := 0; i < 10; i++ {
				simulateSentPayment(t, env, ownerAccount2, isPublic, intent.StateConfirmed)
			}

			// Staff users should not be subject to any denials
			allow, err = env.guard.AllowSendPayment(env.ctx, ownerAccount2, isPublic, testutil.NewRandomAccount(t))
			require.NoError(t, err)
			assert.Equal(t, isStaffUser, allow)
		}
	}
}

func TestAllowReceivePayments_NotPhoneVerified(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		// Account isn't phone verified, so it cannot be used for payments
		for i := 0; i < 5; i++ {
			allow, err := env.guard.AllowReceivePayments(env.ctx, testutil.NewRandomAccount(t), isPublic)
			require.NoError(t, err)
			assert.False(t, allow)
		}
	}
}

func TestAllowReceivePayments_TimeBetweenIntents(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		phoneNumber := "+12223334444"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}

		// First payment should always succeed
		allow, err := env.guard.AllowReceivePayments(env.ctx, ownerAccount1, isPublic)
		require.NoError(t, err)
		assert.True(t, allow)

		// Subsequent payments should fail hitting the time-based rate limit
		// regardless of owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			allow, err = env.guard.AllowReceivePayments(env.ctx, ownerAccount, isPublic)
			require.NoError(t, err)
			assert.False(t, allow)
		}

		// After waiting the timeout, the payments should be allowed regardless of
		// owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			// todo: need a better way to test with time than waiting
			time.Sleep(time.Second)

			allow, err = env.guard.AllowReceivePayments(env.ctx, ownerAccount, isPublic)
			require.NoError(t, err)
			assert.True(t, allow)
		}
	}
}

func TestAllowReceivePayments_TimeBasedLimit(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		phoneNumber := "+12223334444"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}

		// First payment should always succeed
		allow, err := env.guard.AllowReceivePayments(env.ctx, ownerAccount1, isPublic)
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the daily limit of payments
		for _, state := range []intent.State{
			intent.StateUnknown,
			intent.StatePending,
			intent.StateFailed,
			intent.StateConfirmed,
		} {
			for i := 0; i < 2; i++ {
				simulateReceivedPayment(t, env, ownerAccount1, isPublic, state)
			}
		}

		// Payments are denied after breaching the daily count limit regardless of
		// owner account associated with the phone number
		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			// todo: need a better way to test with time than waiting
			time.Sleep(time.Second)

			allow, err := env.guard.AllowReceivePayments(env.ctx, ownerAccount, isPublic)
			require.NoError(t, err)
			assert.False(t, allow)
		}
	}
}

func TestAllowReceivePayments_StaffUser(t *testing.T) {
	for _, isPublic := range []bool{true, false} {
		env := setup(t)

		for i, isStaffUser := range []bool{true, false} {
			phoneNumber := fmt.Sprintf("+1800555000%d", i)

			ownerAccount1 := testutil.NewRandomAccount(t)
			ownerAccount2 := testutil.NewRandomAccount(t)

			require.NoError(t, env.data.PutUser(env.ctx, &identity.Record{
				ID: user.NewUserID(),
				View: &user.View{
					PhoneNumber: &phoneNumber,
				},
				IsStaffUser: isStaffUser,
				CreatedAt:   time.Now(),
			}))

			for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {

				verification := &phone.Verification{
					PhoneNumber:    phoneNumber,
					OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
					CreatedAt:      time.Now(),
					LastVerifiedAt: time.Now(),
				}
				require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
			}

			// First payment should always be successful, regardless of user status
			allow, err := env.guard.AllowReceivePayments(env.ctx, ownerAccount1, isPublic)
			require.NoError(t, err)
			assert.True(t, allow)

			// Consume the remaining daily limits for payments.
			for i := 0; i < 10; i++ {
				simulateReceivedPayment(t, env, ownerAccount2, isPublic, intent.StateConfirmed)
			}

			// Staff users should not be subject to any denials
			allow, err = env.guard.AllowReceivePayments(env.ctx, ownerAccount2, isPublic)
			require.NoError(t, err)
			assert.Equal(t, isStaffUser, allow)
		}
	}
}

func TestAllowOpenAccounts_HappyPath(t *testing.T) {
	for _, testCase := range []intent.State{intent.StateUnknown, intent.StatePending, intent.StateConfirmed} {
		env := setup(t)

		phoneNumber := "+18005550000"

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		// Account isn't phone verified, so it cannot be created
		for i := 0; i < 5; i++ {
			allow, _, _, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, pointer.String(memory_device_verifier.ValidDeviceToken))
			require.NoError(t, err)
			assert.False(t, allow)
		}

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			verification := &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}
			require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
		}

		// New accounts are always denied when using a fake or unverifiable device.
		for i := 0; i < 5; i++ {
			allow, reason, _, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, pointer.String(memory_device_verifier.InvalidDeviceToken))
			require.NoError(t, err)
			assert.False(t, allow)
			assert.Equal(t, ReasonUnsupportedDevice, reason)

			allow, reason, _, err = env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, nil)
			require.NoError(t, err)
			assert.False(t, allow)
			assert.Equal(t, ReasonUnsupportedDevice, reason)
		}

		// The first account creation should always be successful
		allow, _, successCallback, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, pointer.String(memory_device_verifier.ValidDeviceToken))
		require.NoError(t, err)
		assert.True(t, allow)
		require.NotNil(t, successCallback)

		// Have a set of account creations that are in an unsuccessful terminal
		// state that won't be fixed
		for _, state := range []intent.State{intent.StateRevoked} {
			simulateAccountCreation(t, env, ownerAccount1, state, time.Now())
		}

		// Account creation is unaffected by previous creations that didn't
		// result in success
		allow, _, _, err = env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, pointer.String(memory_device_verifier.ValidDeviceToken))
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the remaining lifetime limit of account creations
		simulateAccountCreation(t, env, ownerAccount1, testCase, time.Now().Add(-10*365*24*time.Hour))

		// Account creations are denied after breaching the daily count limit regardless
		// of owner account associated with the phone number
		for i := 0; i < 5; i++ {
			allow, reason, _, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount2, pointer.String(memory_device_verifier.ValidDeviceToken))
			require.NoError(t, err)
			assert.False(t, allow)
			assert.Equal(t, ReasonTooManyFreeAccountsForPhoneNumber, reason)
		}

		// New accounts are always denied within the same device
		require.NoError(t, successCallback())

		newPhoneNumber := "+11234567890"
		verification := &phone.Verification{
			PhoneNumber:    newPhoneNumber,
			OwnerAccount:   ownerAccount2.PublicKey().ToBase58(),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		}
		require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))

		for i := 0; i < 5; i++ {
			allow, reason, _, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount2, pointer.String(memory_device_verifier.ValidDeviceToken))
			require.NoError(t, err)
			assert.False(t, allow)
			assert.Equal(t, ReasonTooManyFreeAccountsForDevice, reason)
		}
	}
}

func TestAllowOpenAccounts_StaffUser(t *testing.T) {
	env := setup(t)

	for i, isStaffUser := range []bool{true, false} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		require.NoError(t, env.data.PutUser(env.ctx, &identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			IsStaffUser: isStaffUser,
			CreatedAt:   time.Now(),
		}))

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {

			verification := &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}
			require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
		}

		// First account creation is always successful, regardless of user status
		allow, _, _, err := env.guard.AllowOpenAccounts(env.ctx, ownerAccount1, pointer.String(memory_device_verifier.ValidDeviceToken))
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the remaining daily limit of account creations
		simulateAccountCreation(t, env, ownerAccount1, intent.StateConfirmed, time.Now())

		// Staff users should not be subject to any denials
		allow, _, _, err = env.guard.AllowOpenAccounts(env.ctx, ownerAccount2, pointer.String(memory_device_verifier.ValidDeviceToken))
		require.NoError(t, err)
		assert.Equal(t, isStaffUser, allow)
	}
}

func TestAllowEstablishNewRelationship_HappyPath(t *testing.T) {
	env := setup(t)

	for i, testCase := range []intent.State{intent.StateUnknown, intent.StatePending, intent.StateConfirmed} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		// Account isn't phone verified, so it cannot be created
		for i := 0; i < 5; i++ {
			allow, err := env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount1, "getcode.com")
			require.NoError(t, err)
			assert.False(t, allow)
		}

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			verification := &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}
			require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
		}

		// Daily limit not consumed
		allow, err := env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount1, "getcode.com")
		require.NoError(t, err)
		assert.True(t, allow)

		// Have a set of new relationships that are revoked
		for i := 0; i < 10; i++ {
			simulateRelationshipEstablished(t, env, ownerAccount1, intent.StateRevoked, time.Now())
		}

		// Limit is unaffected
		allow, err = env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount1, "getcode.com")
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the remaining limit
		for i := 0; i < 10; i++ {
			simulateRelationshipEstablished(t, env, ownerAccount1, testCase, time.Now())
		}

		// New relationships are denied after breaching the daily count limit regardless
		// of owner account associated with the phone number
		for i := 0; i < 5; i++ {
			allow, err := env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount2, "getcode.com")
			require.NoError(t, err)
			assert.False(t, allow)
		}
	}
}

func TestAllowEstablishNewRelationship_StaffUser(t *testing.T) {
	env := setup(t)

	for i, isStaffUser := range []bool{true, false} {
		phoneNumber := fmt.Sprintf("+1800555000%d", i)

		ownerAccount1 := testutil.NewRandomAccount(t)
		ownerAccount2 := testutil.NewRandomAccount(t)

		require.NoError(t, env.data.PutUser(env.ctx, &identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			IsStaffUser: isStaffUser,
			CreatedAt:   time.Now(),
		}))

		for _, ownerAccount := range []*common.Account{ownerAccount1, ownerAccount2} {
			verification := &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}
			require.NoError(t, env.guard.data.SavePhoneVerification(env.ctx, verification))
		}

		// Daily limit not consumed
		allow, err := env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount2, "getcode.com")
		require.NoError(t, err)
		assert.True(t, allow)

		// Consume the remaining limit
		for i := 0; i < 10; i++ {
			simulateRelationshipEstablished(t, env, ownerAccount1, intent.StatePending, time.Now())
		}

		// Staff users should not be subject to any denials
		allow, err = env.guard.AllowEstablishNewRelationship(env.ctx, ownerAccount2, "getcode.com")
		require.NoError(t, err)
		assert.Equal(t, isStaffUser, allow)
	}
}

func simulateSentPayment(t *testing.T, env testEnv, ownerAccount *common.Account, isPublic bool, state intent.State) {
	verificationRecord, err := env.data.GetLatestPhoneVerificationForAccount(env.ctx, ownerAccount.PublicKey().ToBase58())
	require.NoError(t, err)

	if isPublic {
		intentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPublicPayment,
			SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
				DestinationOwnerAccount: "destination_owner",
				DestinationTokenAccount: "destination_token",
				Quantity:                1,
				ExchangeCurrency:        currency.KIN,
				ExchangeRate:            1,
				NativeAmount:            1,
				UsdMarketValue:          1,
			},
			InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
			InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
			State:                 state,
			CreatedAt:             time.Now(),
		}
		require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
	} else {
		intentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.SendPrivatePayment,
			SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{
				DestinationOwnerAccount: "destination_owner",
				DestinationTokenAccount: "destination_token",
				Quantity:                1,
				ExchangeCurrency:        currency.KIN,
				ExchangeRate:            1,
				NativeAmount:            1,
				UsdMarketValue:          1,
			},
			InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
			InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
			State:                 state,
			CreatedAt:             time.Now(),
		}
		require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
	}
}

func simulateReceivedPayment(t *testing.T, env testEnv, ownerAccount *common.Account, isPublic bool, state intent.State) {
	verificationRecord, err := env.data.GetLatestPhoneVerificationForAccount(env.ctx, ownerAccount.PublicKey().ToBase58())
	require.NoError(t, err)

	if isPublic {
		intentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.ReceivePaymentsPublicly,
			ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
				Source:                   "gift_card",
				Quantity:                 1,
				IsRemoteSend:             true,
				OriginalExchangeCurrency: currency.KIN,
				OriginalExchangeRate:     1.0,
				OriginalNativeAmount:     1.0,
				UsdMarketValue:           1,
			},
			InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
			InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
			State:                 state,
			CreatedAt:             time.Now(),
		}
		require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
	} else {
		intentRecord := &intent.Record{
			IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
			IntentType: intent.ReceivePaymentsPrivately,
			ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{
				Source:         "source",
				Quantity:       1,
				UsdMarketValue: 1,
			},
			InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
			InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
			State:                 state,
			CreatedAt:             time.Now(),
		}
		require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
	}
}

func simulateAccountCreation(t *testing.T, env testEnv, ownerAccount *common.Account, state intent.State, createdAt time.Time) {
	verificationRecord, err := env.data.GetLatestPhoneVerificationForAccount(env.ctx, ownerAccount.PublicKey().ToBase58())
	require.NoError(t, err)

	intentRecord := &intent.Record{
		IntentId:              testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType:            intent.OpenAccounts,
		OpenAccountsMetadata:  &intent.OpenAccountsMetadata{},
		InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
		State:                 state,
		CreatedAt:             createdAt,
	}
	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
}

func simulateRelationshipEstablished(t *testing.T, env testEnv, ownerAccount *common.Account, state intent.State, createdAt time.Time) {
	verificationRecord, err := env.data.GetLatestPhoneVerificationForAccount(env.ctx, ownerAccount.PublicKey().ToBase58())
	require.NoError(t, err)

	intentRecord := &intent.Record{
		IntentId:   testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.EstablishRelationship,
		EstablishRelationshipMetadata: &intent.EstablishRelationshipMetadata{
			RelationshipTo: "example.com",
		},
		InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),
		InitiatorPhoneNumber:  &verificationRecord.PhoneNumber,
		State:                 state,
		CreatedAt:             createdAt,
	}
	require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
}
*/
