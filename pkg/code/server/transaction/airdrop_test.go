package transaction_v2

/*
import (
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestAirdrop_GetFirstKin_HappyPath(t *testing.T) {
	for _, clearCache := range []bool{true, false} {
		server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_5000_000_000))
		server.generateAvailableNonces(t, 1000)

		phone.openAccounts(t).requireSuccess(t)

		resp := phone.requestAirdrop(t, transactionpb.AirdropType_GET_FIRST_KIN)
		assert.Equal(t, transactionpb.AirdropResponse_OK, resp.Result)
		assert.EqualValues(t, currency_lib.USD, resp.ExchangeData.Currency)
		assert.Equal(t, 0.1, resp.ExchangeData.ExchangeRate)
		assert.Equal(t, 1.0, resp.ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(10)+kin.ToQuarks(1), resp.ExchangeData.Quarks)
		server.assertAirdroppedFirstKin(t, phone)

		for i := 0; i < 10; i++ {
			if clearCache {
				cachedAirdropStatus.Clear()
			}

			resp = phone.requestAirdrop(t, transactionpb.AirdropType_GET_FIRST_KIN)
			assert.Equal(t, transactionpb.AirdropResponse_ALREADY_CLAIMED, resp.Result)
		}
		phone.assertAirdropCount(t, 1)
	}
}

func TestAirdrop_GetFirstKin_InvalidOwner(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAirdrops: true,
	})
	defer cleanup()

	server.setupAirdropper(t, kin.ToQuarks(1_5000_000_000))
	server.generateAvailableNonces(t, 1000)

	resp := phone.requestAirdrop(t, transactionpb.AirdropType_GET_FIRST_KIN)
	assert.Equal(t, transactionpb.AirdropResponse_UNAVAILABLE, resp.Result)
	server.assertNotAirdroppedFirstKin(t, phone)
}

func TestAirdrop_GetFirstKin_InsufficientBalance(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAirdrops: true,
	})
	defer cleanup()

	server.setupAirdropper(t, kin.ToQuarks(1))
	server.generateAvailableNonces(t, 1000)

	phone.openAccounts(t).requireSuccess(t)

	resp := phone.requestAirdrop(t, transactionpb.AirdropType_GET_FIRST_KIN)
	assert.Equal(t, transactionpb.AirdropResponse_UNAVAILABLE, resp.Result)
	server.assertNotAirdroppedFirstKin(t, phone)
}


func TestAirdrop_GiveFirstKin_CodeToCodePayment(t *testing.T) {
	for _, clearCache := range []bool{true, false} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
		server.generateAvailableNonces(t, 10000)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		var intentIds []string
		for i := 0; i < 200; i++ {
			if clearCache {
				cachedAirdropStatus.Clear()
				cachedFirstReceivesByOwner.Clear()
			}

			submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
			submitIntentCall.requireSuccess(t)

			intentIds = append(intentIds, submitIntentCall.intentId)

			receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)
		}

		for i, intentId := range intentIds {
			if i == 0 {
				server.assertAirdroppedForGivingFirstKin(t, sendingPhone, intentId)
			} else {
				server.assertNotAirdroppedForGivingFirstKin(t, intentId)
			}
		}
	}
}

func TestAirdrop_GiveFirstKin_CodeToCodePublicWithdrawal(t *testing.T) {
	for _, clearCache := range []bool{true, false} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
		server.generateAvailableNonces(t, 10000)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		var intentIds []string
		for i := 0; i < 10; i++ {
			if clearCache {
				cachedAirdropStatus.Clear()
				cachedFirstReceivesByOwner.Clear()
			}

			submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUser(t, receivingPhone)
			submitIntentCall.requireSuccess(t)

			intentIds = append(intentIds, submitIntentCall.intentId)
		}

		for i, intentId := range intentIds {
			if i == 0 {
				server.assertAirdroppedForGivingFirstKin(t, sendingPhone, intentId)
			} else {
				server.assertNotAirdroppedForGivingFirstKin(t, intentId)
			}
		}
	}
}

func TestAirdrop_GiveFirstKin_CodeToCodePrivateWithdrawal(t *testing.T) {
	for _, clearCache := range []bool{true, false} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
		server.generateAvailableNonces(t, 10000)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		var intentIds []string
		for i := 0; i < 10; i++ {
			if clearCache {
				cachedAirdropStatus.Clear()
				cachedFirstReceivesByOwner.Clear()
			}

			submitIntentCall := sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
			submitIntentCall.requireSuccess(t)

			intentIds = append(intentIds, submitIntentCall.intentId)
		}

		for i, intentId := range intentIds {
			if i == 0 {
				server.assertAirdroppedForGivingFirstKin(t, sendingPhone, intentId)
			} else {
				server.assertNotAirdroppedForGivingFirstKin(t, intentId)
			}
		}
	}
}

func TestAirdrop_GiveFirstKin_RemoteSend(t *testing.T) {
	for _, clearCache := range []bool{true, false} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
		server.generateAvailableNonces(t, 10000)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		var intentIds []string
		for i := 0; i < 10; i++ {
			if clearCache {
				cachedAirdropStatus.Clear()
				cachedFirstReceivesByOwner.Clear()
			}

			giftCard := testutil.NewRandomAccount(t)

			sendingPhone.send42KinToGiftCardAccount(t, giftCard).requireSuccess(t)
			submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCard, false)
			submitIntentCall.requireSuccess(t)
			receivingPhone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)

			intentIds = append(intentIds, submitIntentCall.intentId)
		}

		for i, intentId := range intentIds {
			if i == 0 {
				server.assertAirdroppedForGivingFirstKin(t, sendingPhone, intentId)
			} else {
				server.assertNotAirdroppedForGivingFirstKin(t, intentId)
			}
		}
	}
}

func TestAirdrop_GiveFirstKin_PrivacyMigration(t *testing.T) {
	for _, amountToMigrate := range []uint64{
		0,
		kin.ToQuarks(23),
	} {
		cachedAirdropStatus.Clear()

		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
			enableAirdrops: true,
		})
		defer cleanup()

		server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
		server.generateAvailableNonces(t, 1000)
		cachedFirstReceivesByOwner.Clear()

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		legacyTimelockVault, err := receivingPhone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
		require.NoError(t, err)

		if amountToMigrate > 0 {
			server.fundAccount(t, legacyTimelockVault, amountToMigrate)
		}

		receivingPhone.migrateToPrivacy2022(t, amountToMigrate).requireSuccess(t)

		submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
		submitIntentCall.requireSuccess(t)

		if amountToMigrate > 0 {
			server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)
		} else {
			server.assertAirdroppedForGivingFirstKin(t, sendingPhone, submitIntentCall.intentId)
		}
	}
}

func TestAirdrop_GiveFirstKin_EdgeCasesAndIntentsThatDontCount(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
		enableAirdrops: true,
	})
	defer cleanup()

	server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
	server.generateAvailableNonces(t, 1000)

	submitIntentCall := sendingPhone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	// Simulate an external deposit

	server.simulateExternalDepositHistoryItem(t, receivingPhone.parentAccount, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), kin.ToQuarks(100))

	// Iterate over various types of payments coming from the receiver before
	// they get their first Kin from someone else. Importantly, some of these
	// also generate airdrops for the receiver.

	submitIntentCall = receivingPhone.send42KinToCodeUser(t, sendingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertAirdroppedForGivingFirstKin(t, receivingPhone, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.privatelyWithdraw777KinToCodeUser(t, sendingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.publiclyWithdraw777KinToCodeUser(t, sendingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	// Create some gift cards with all iterations of interactions within the
	// same user.

	for _, isGiftCardVoided := range []bool{true, false} {
		giftCard := testutil.NewRandomAccount(t)

		submitIntentCall = receivingPhone.send42KinToGiftCardAccount(t, giftCard)
		submitIntentCall.requireSuccess(t)
		server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

		submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCard, isGiftCardVoided)
		submitIntentCall.requireSuccess(t)
		server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

		submitIntentCall = receivingPhone.receive42KinPrivatelyIntoOrganizer(t)
		submitIntentCall.requireSuccess(t)
		server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)
	}

	giftCard := testutil.NewRandomAccount(t)
	submitIntentCall = receivingPhone.send42KinToGiftCardAccount(t, giftCard)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)

	// Finally, have the user receive their first Kin from someone else, which
	// should still end up with a airdrop to the sender despite everything that
	// happened prior this intent.

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertAirdroppedForGivingFirstKin(t, sendingPhone, submitIntentCall.intentId)
}

func TestAirdrop_GiveFirstKin_InsufficientAirdropperFunds(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
		enableAirdrops: true,
	})
	defer cleanup()

	server.setupAirdropper(t, kin.ToQuarks(1))
	server.generateAvailableNonces(t, 10000)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertNotAirdroppedForGivingFirstKin(t, submitIntentCall.intentId)
}


func TestAirdrop_IntentId(t *testing.T) {
	reference1 := testutil.NewRandomAccount(t).PublicKey().ToBase58()
	reference2 := testutil.NewRandomAccount(t).PublicKey().ToBase58()

	generated1 := GetNewAirdropIntentId(AirdropTypeGetFirstKin, reference1)
	generated2 := GetNewAirdropIntentId(AirdropTypeGiveFirstKin, reference1)
	generated3 := GetNewAirdropIntentId(AirdropTypeGetFirstKin, reference2)

	assert.NotEqual(t, reference1, generated1)
	assert.NotEqual(t, reference1, generated2)
	assert.NotEqual(t, reference2, generated3)
	assert.NotEqual(t, generated1, generated2)
	assert.NotEqual(t, generated2, generated3)

	for _, generated := range []string{generated1, generated2, generated3} {
		decoded, err := base58.Decode(generated)
		require.NoError(t, err)
		assert.Len(t, decoded, 32)
	}

	for i := 0; i < 100; i++ {
		assert.Equal(t, generated1, GetNewAirdropIntentId(AirdropTypeGetFirstKin, reference1))
		assert.Equal(t, generated2, GetNewAirdropIntentId(AirdropTypeGiveFirstKin, reference1))
		assert.Equal(t, generated3, GetNewAirdropIntentId(AirdropTypeGetFirstKin, reference2))
	}
}
*/
