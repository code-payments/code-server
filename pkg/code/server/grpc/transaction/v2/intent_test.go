package transaction_v2

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestPrivacyV3Demonstration(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Initialize new user accounts
	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	// Make a private payment from sender to receiver
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)

	// Do a public withdraw from sender to receiver, where the sender funds
	// their primary account from their organizer via a private withdraw.
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, sendingPhone).requireSuccess(t)
	sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone).requireSuccess(t)

	// Simulate transfers from the treasury and subsequent updates to the merkle tree
	server.simulateTreasuryPayments(t)

	// Check with server for privacy upgrades (there shouldn't be any)
	assert.False(t, sendingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))
	assert.False(t, receivingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))

	// Simulate more transactions which can be a target for privacy upgrades
	sendingPhone.publiclyWithdraw123KinToExternalWallet(t).requireSuccess(t)
	sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)
	for i := 0; i < 3; i++ {
		giftCardAccount := testutil.NewRandomAccount(t)
		sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)
		receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false).requireSuccess(t)
		receivingPhone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)
	}
	server.simulateTreasuryPayments(t)

	// Check with server for privacy upgrades (there should be some now)
	require.True(t, sendingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))
	require.True(t, receivingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))

	// Upgrade a transaction to permanent privacy for each phone
	sendingPhone.upgradeOneTxnToPermanentPrivacy(t, true).requireSuccess(t)
	receivingPhone.upgradeOneTxnToPermanentPrivacy(t, true).requireSuccess(t)
}

func TestSubmitIntent_OpenAccounts_HappyPath(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, phone, nil)
}

func TestSubmitIntent_OpenAccounts_AntispamGuard(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	// Resetting phone so it can open a new set of accounts
	phone.reset(t)
	server.phoneVerifyUser(t, phone)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertDeniedResponse(t, "too many account creations")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_OpenAccounts_MultipleAttempts(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_OpenAccounts_Validation_Actions(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	//
	// Part 1: Opening incorrect number of accounts
	//

	phone.resetConfig()
	phone.conf.simulateOpeningTooFewAccounts = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 19 total actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateOpeningTooManyAccounts = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 19 total actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Validation for action to open primary account
	//

	phone.resetConfig()
	phone.conf.simulateOpenPrimaryAccountActionReplaced = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: expected an open account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAccountTypeForOpenPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: account type must be PRIMARY")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAuthorityForOpenPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[0]: authority must be %s", phone.parentAccount.PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidTokenAccountForOpenPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[0]: token must be %s", phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidIndexForOpenPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: index must be 0 for all newly opened accounts")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Validation for action to open non-primary account
	//

	phone.resetConfig()
	phone.conf.simulateOpenNonPrimaryAccountActionReplaced = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[1]: expected an open account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAccountTypeForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[1]: account type must be TEMPORARY_INCOMING")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[1]: authority cannot be %s", phone.parentAccount.PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidTokenAccountForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[1]: token must be %s", phone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidIndexForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[1]: index must be 0 for all newly opened accounts")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 4: Validation for action to close dormant account
	//

	phone.resetConfig()
	phone.conf.simulateCloseDormantAccountActionReplaced = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: expected a close dormant account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAccountTypeForCloseDormantAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: expected TEMPORARY_INCOMING account type")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAuthorityForCloseDormantAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[2]: authority must be %s", phone.getAuthority(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidTokenAccountForCloseDormantAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[2]: token must be %s", phone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidDestitinationForCloseDormantAccountAction = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[2]: destination must be %s", phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 5: Unnecessary fee payments
	//

	phone.resetConfig()
	phone.conf.simulateFeePaid = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 19 total actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatePayment_DirectlyBetweenCodeUsers_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPrivatePayment_WithdrawToCodeUser_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPrivatePayment_WithdrawToExternalWallet_HappyPath(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)
}

func TestSubmitIntent_SendPrivatePayment_WithdrawToRelationshipAccountForMerchantPayment_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.establishRelationshipWithMerchant(t, domain)

	submitIntentCall := sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPrivatePayment_RemoteSend_HappyPath(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	submitIntentCall := sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)
	server.assertRemoteSendGiftCardAccountRecordsSaved(t, giftCardAccount)
}

func TestSubmitIntent_SendPrivatePayment_RemoteSend_UseExistingGiftCard(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	submitIntentCall := sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount)
	submitIntentCall.assertStaleStateResponse(t, "actions[0]: account is already opened")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatepayment_RemoteSend_NoWithdrawals(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	submitIntentCall := sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "withdrawal and remote send flags cannot both be true set at the same time")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatePayment_PaymentRequest_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	sendingPhone.conf.simulatePaymentRequest = true

	submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)

	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)

	submitIntentCall = sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPrivatePayment_SelfPayment(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	sendingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	submitIntentCall := sendingPhone.send42KinToCodeUser(t, sendingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payments within the same owner are not allowed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, sendingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &sendingPhone)

	submitIntentCall = sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, sendingPhone, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &sendingPhone)
}

func TestSubmitIntent_SendPrivatePayment_AntispamGuard(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 10; i++ {
		submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
		if submitIntentCall.isError(t) {
			submitIntentCall.assertDeniedResponse(t, "too many payments")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
			return
		}
	}

	assert.Fail(t, "antispam guard not triggered")
}

func TestSubmitIntent_SendPrivatePayment_AntiMoneyLaunderingGuard(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAmlChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.privatelyWithdrawMillionDollarsToExternalWallet(t)
	submitIntentCall.assertDeniedResponse(t, "dollar value exceeds limit")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatePayment_Validation_ManagedByCode(t *testing.T) {
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		for _, accountType := range accountTypesToOpen {
			server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
			defer cleanup()

			server.generateAvailableNonces(t, 100)

			sendingPhone.openAccounts(t).requireSuccess(t)

			server.simulateTimelockAccountInState(t, sendingPhone.getTimelockVault(t, accountType, 0), state)

			submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
		}
	}
}

func TestSubmitIntent_SendPrivatePayment_Validation_ExchangeData(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	sendingPhone.conf.simulateInvalidExchangeRate = true

	submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertStaleStateResponse(t, "fiat exchange rate is stale")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "kin exchange rate must be 1")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidNativeAmount = true

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment native amount and quark value mismatch")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatePayment_Validation_Balances(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Resetting phone to a state where there are no balances
	sendingPhone.reset(t)
	server.phoneVerifyUser(t, sendingPhone)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "insufficient balance to perform action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPrivatePayment_Validation_Actions(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)

	//
	// Part 1: Validate quantity of funds being sent
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendingTooLittle = true
	submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send 4200001 quarks to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendingTooMuch = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send 4199999 quarks to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFundingTempAccountTooMuch = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must fund temporary outgoing account with 4200000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Validate destination account
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "destination account must be a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "destination account must be a temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "destination account must be a temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "destination account must be a temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "remote send must be to a brand new gift card account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	sendingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "remote send must be to a brand new gift card account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	sendingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must open two accounts")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	sendingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "must open one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Validate how funds are sent to destination
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPrivatePaymentFromPreviousTempOutgoingAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment must be sent from temporary outgoing account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPrivatePaymentFromTempIncomingAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment must be sent from temporary outgoing account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPrivatePaymentFromBucketAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment must be sent from temporary outgoing account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPrivatePaymentWithoutClosingTempIncomingAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment sent to destination must be a public withdraw")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 4: Validate temporary account rotation
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotClosingTempAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateClosingWrongAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: payment sent to destination must be a public withdraw")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOpeningWrongTempAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "open account action for TEMPORARY_OUTGOING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidAccountTypeForOpenNonPrimaryAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "open account action for TEMPORARY_OUTGOING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertStaleStateResponse(t, "actions[4]: account is already opened")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForOpenNonPrimaryAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidIndexForOpenNonPrimaryAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: next derivation expected to be 2")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidAccountTypeForCloseDormantAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "close dormant account action for TEMPORARY_OUTGOING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidAuthorityForCloseDormantAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[5]: authority must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForCloseDormantAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[5]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidDestitinationForCloseDormantAccountAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[5]: destination must be %s", getTimelockVault(t, sendingPhone.parentAccount).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 5: Validate account types that can send/receive funds
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingPrimaryAccountAsSource = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: primary account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingCurrentTempIncomingAccountAsDestination = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: temporary incoming account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingNewTempAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: new temporary outgoing account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPrivatePaymentWithMoreTempOutgoingOutboundTransfers = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "temporary outgoing account can only send kin 1 time")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "destination account can only receive funds in one action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: source is not a latest owned account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[7]: source is not a latest owned account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[7]: source is not a latest owned account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	//	Part 6: Validate bucket account usage
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidBucketExchangeMultiple = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: quark amount must be a multiple")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidAnyonymizedBucketAmount = true
	server.fundAccount(t, sendingPhone.getTimelockVault(t, commonpb.AccountType_BUCKET_10_000_KIN, 0), kin.ToQuarks(10_000_000))
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: quark amount must be anonymized")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateBucketExchangeInATransfer = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "expected an exchange action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateBucketExchangeInAPublicTransfer = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "bucket account cannot send/receive kin publicly")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateBucketExchangeInAPublicWithdraw = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must close one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSwappingTransfersForExchanges = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: destination account must be an owned bucket account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSwappingExchangesForTransfers = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[3]: expected an exchange action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 7: Validate accounts used in money movement actions
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForNoPrivacyWithdrawAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForTemporaryPrivacyTransferAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForTemporaryPrivacyExchangeAction = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[3]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 8: Validate number of money movement actions
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateTooManyTemporaryPrivacyTransfers = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertDeniedResponse(t, "too many transfer/exchange/withdraw actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateTooManyTemporaryPrivacyExchanges = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertDeniedResponse(t, "too many transfer/exchange/withdraw actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 9: Inject random actions that don't belong
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulatePrivacyUpgradeActionInjected = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: update action not allowed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomOpenAccountActionInjected = true

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must open one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "must open two accounts")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomCloseEmptyAccountActionInjected = true
	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must close one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomCloseDormantAccountActionInjected = true

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "too many close dormant account actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "too many close dormant account actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 10: Validate gift card account opening
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotOpeningGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "must open two accounts")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOpeningWrongGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOpeningGiftCardWithWrongOwner = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: owner must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOpeningGiftCardWithUser12Words = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertStaleStateResponse(t, "actions[0]: account is already opened")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateOpeningGiftCardAsWrongAccountType = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "open account action for REMOTE_SEND_GIFT_CARD account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateClosingGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "actions[8]: attempt to close an account with a non-zero balance")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotClosingDormantGiftCardAccount = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "close dormant account action for REMOTE_SEND_GIFT_CARD account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateClosingDormantGiftCardAsWrongAccountType = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "close dormant account action for REMOTE_SEND_GIFT_CARD account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidGiftCardIndex = true
	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: index must be 0")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 11: Unnecessary fee payments
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFeePaid = true

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPublicPayment_WithdrawToCodeUser_FromPrimaryToPrimary_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPublicPayment_WithdrawToCodeUser_FromRelationshipToRelationship_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)
	receivingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	server.fundAccount(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(10_000))

	submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUserBetweenRelationshipAccounts(t, domain, receivingPhone)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, &receivingPhone)
}

func TestSubmitIntent_SendPublicPayment_WithdrawToExternalWallet_FromPrimary_HappyPath(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)
}

func TestSubmitIntent_SendPublicPayment_WithdrawToExternalWallet_FromRelationship_HappyPath(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	sendingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	server.fundAccount(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(1_000))

	submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWalletFromRelationshipAccount(t, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)
}

func TestSubmitIntent_SendPublicPayment_SelfPayment(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"
	sendingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	server.fundAccount(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(1_000))

	submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, sendingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payments within the same owner are not allowed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenRelationshipAccounts(t, domain, sendingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payments within the same owner are not allowed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_SendPublicPayment_AntispamGuard(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 10; i++ {
		submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
		if submitIntentCall.isError(t) {
			submitIntentCall.assertDeniedResponse(t, "too many payments")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

			return
		}
	}

	assert.Fail(t, "antispam guard not triggered")
}

func TestSubmitIntent_SendPublicPayment_Validation_ManagedByCode(t *testing.T) {
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		for _, accountType := range accountTypesToOpen {
			server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
			defer cleanup()

			server.generateAvailableNonces(t, 100)

			sendingPhone.openAccounts(t).requireSuccess(t)

			server.simulateTimelockAccountInState(t, sendingPhone.getTimelockVault(t, accountType, 0), state)

			submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
		}

		server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		domain := "getcode.com"

		sendingPhone.openAccounts(t).requireSuccess(t)
		sendingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

		server.fundAccount(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(1_000))
		server.simulateTimelockAccountInState(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, domain)), state)

		submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
		submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	}
}

func TestSubmitIntent_SendPublicPayment_Validation_ExchangeData(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	sendingPhone.conf.simulateInvalidExchangeRate = true

	submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertStaleStateResponse(t, "fiat exchange rate is stale")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "kin exchange rate must be 1")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidNativeAmount = true

	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment native amount and quark value mismatch")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_SendPublicPayment_Validation_Balances(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Resetting phone to a state where there are no balances
	sendingPhone.reset(t)
	server.phoneVerifyUser(t, sendingPhone)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "insufficient balance to perform action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_SendPublicPayment_Validation_Actions(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	//
	// Part 1: Validate quantity of funds being sent
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendingTooLittle = true
	submitIntentCall := sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send 77700001 quarks to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendingTooMuch = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send 77699999 quarks to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Validate destination account
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment to destination account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Validate withdraw flag
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFlippingWithdrawFlag = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 4: Validate mechanisms funds are transferred between source and destination
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPublicPaymentPrivately = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment sent to destination must be a public transfer")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPublicPaymentWithWithdraw = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "payment sent to destination must be a public transfer")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 5: Validate accounts used in money movement actions
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateNotSendingFromSource = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "must send payment from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateSendPublicPaymentFromBucketAccount = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "source account must be a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "source account must be a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidTokenAccountForNoPrivacyTransferAction = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 6: Inject random actions that don't belong
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulatePrivacyUpgradeActionInjected = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomOpenAccountActionInjected = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomCloseEmptyAccountActionInjected = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateRandomCloseDormantAccountActionInjected = true
	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 7: Unnecessary fee payments
	//

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateFeePaid = true

	submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_ReceivePaymentsPrivately_FromCodeToCodePayment_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)

	submitIntentCall := receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, receivingPhone, nil)
}

func TestSubmitIntent_ReceivePaymentsPrivately_FromRemoteSendGiftCardPublicReceive_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)
	receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false).requireSuccess(t)

	submitIntentCall := receivingPhone.receive42KinPrivatelyIntoOrganizer(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, receivingPhone, nil)
}

func TestSubmitIntent_ReceivePaymentsPrivately_FromDeposit_PrimaryAccount_HappyPath(t *testing.T) {
	server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, receivingPhone, nil)
}

func TestSubmitIntent_ReceivePaymentsPrivately_FromDeposit_RelationshipAccount_HappyPath(t *testing.T) {
	server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"

	receivingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

	server.fundAccount(t, getTimelockVault(t, receivingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(1_000))

	submitIntentCall := receivingPhone.deposit777KinIntoOrganizerFromRelationshipAccount(t, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, receivingPhone, nil)
}

func TestSubmitIntent_ReceivePaymentsPrivately_AntispamGuard(t *testing.T) {
	server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	receivingPhone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 10; i++ {
		submitIntentCall := receivingPhone.deposit777KinIntoOrganizer(t)
		if submitIntentCall.isError(t) {
			submitIntentCall.assertDeniedResponse(t, "too many payments")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

			return
		}
	}

	assert.Fail(t, "antispam guard not triggered")
}

func TestSubmitIntent_ReceivePaymentsPrivately_FromDeposit_AntiMoneyLaunderingGuard(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAmlChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := sendingPhone.depositMillionDollarsIntoOrganizer(t)
	submitIntentCall.assertDeniedResponse(t, "dollar value exceeds limit")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPrivately_Validation_ManagedByCode(t *testing.T) {
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		for _, accountType := range accountTypesToOpen {
			server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
			defer cleanup()

			server.generateAvailableNonces(t, 100)

			receivingPhone.openAccounts(t).requireSuccess(t)

			server.simulateTimelockAccountInState(t, receivingPhone.getTimelockVault(t, accountType, 0), state)

			submitIntentCall := receivingPhone.deposit777KinIntoOrganizer(t)
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		}

		server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		domain := "getcode.com"

		receivingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.establishRelationshipWithMerchant(t, domain).requireSuccess(t)

		server.fundAccount(t, getTimelockVault(t, receivingPhone.getAuthorityForRelationshipAccount(t, domain)), kin.ToQuarks(1_000))
		server.simulateTimelockAccountInState(t, getTimelockVault(t, receivingPhone.getAuthorityForRelationshipAccount(t, domain)), state)

		submitIntentCall := receivingPhone.deposit777KinIntoOrganizerFromRelationshipAccount(t, domain)
		submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	}
}

func TestSubmitIntent_ReceivePaymentsPrivately_Validation_Balances(t *testing.T) {
	server, _, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Resetting phone to a state where there are no balances
	receivingPhone.reset(t)
	server.phoneVerifyUser(t, receivingPhone)

	receivingPhone.openAccounts(t).requireSuccess(t)

	submitIntentCall := receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "insufficient balance to perform action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPrivately_Validation_Actions(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)

	//
	// Part 1: Validate quantity of funds being received
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivingTooLittle = true
	submitIntentCall := receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive 4200001 quarks from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivingTooMuch = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive 4199999 quarks from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFundingTempAccountTooMuch = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: attempt to close an account with a non-zero balance")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Validate source account
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateNotReceivingFromSource = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive payments from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFlippingDepositFlag = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFlippingDepositFlag = true
	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from latest temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivePaymentFromPreviousTempIncomingAccount = true
	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0), kin.ToQuarks(42))
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "source is not a latest owned account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivePaymentFromTempOutgoingAccount = true

	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, 0), kin.ToQuarks(42))
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from latest temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, 0), kin.ToQuarks(777))
	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivePaymentFromBucketAccount = true

	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from latest temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive from a deposit account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Validate temporary account rotation
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateNotClosingTempAccount = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must close one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateClosingWrongAccount = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must close latest temporary incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateOpeningWrongTempAccount = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "open account action for TEMPORARY_INCOMING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForCloseEmptyAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[3]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidAccountTypeForOpenNonPrimaryAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "open account action for TEMPORARY_INCOMING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertStaleStateResponse(t, "actions[4]: account is already opened")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForOpenNonPrimaryAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidIndexForOpenNonPrimaryAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: next derivation expected to be 2")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidAccountTypeForCloseDormantAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "close dormant account action for TEMPORARY_INCOMING account type missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidAuthorityForCloseDormantAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[5]: authority must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForCloseDormantAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[5]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidDestitinationForCloseDormantAccountAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[5]: destination must be %s", getTimelockVault(t, receivingPhone.parentAccount).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 4: Validate account types that can send/receive funds
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingPrimaryAccountAsSource = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: deposit account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingPrimaryAccountAsDestination = true
	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: deposit account cannot receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingCurrentTempOutgoingAccountAsDestination = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: temporary outgoing account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingCurrentTempIncomingAccountAsSource = true
	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: temporary incoming account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingCurrentTempIncomingAccountAsDestination = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: temporary incoming account cannot receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingNewTempAccount = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: new temporary incoming account cannot send/receive kin")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingGiftCardAccount = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: source is not a latest owned account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 5: Validate bucket account usage
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidBucketExchangeMultiple = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: quark amount must be a multiple")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidAnyonymizedBucketAmount = true
	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_BUCKET_10_000_KIN, 0), kin.ToQuarks(10_000_000))
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: quark amount must be anonymized")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateBucketExchangeInATransfer = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "expected an exchange action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateBucketExchangeInAPublicTransfer = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "bucket account cannot send/receive kin publicly")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateBucketExchangeInAPublicWithdraw = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must close one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.conf.simulateBucketExchangeInAPublicWithdraw = true
	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: cannot close any account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateSwappingTransfersForExchanges = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: source account must be an owned bucket account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateSwappingExchangesForTransfers = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: expected an exchange action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 6: Validate accounts used in money movement actions
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForTemporaryPrivacyTransferAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForTemporaryPrivacyExchangeAction = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[2]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 7: Validate number of money movement actions
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateTooManyTemporaryPrivacyTransfers = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertDeniedResponse(t, "too many transfer/exchange/withdraw actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateTooManyTemporaryPrivacyExchanges = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertDeniedResponse(t, "too many transfer/exchange/withdraw actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 8: Inject random actions that don't belong
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulatePrivacyUpgradeActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[6]: update action not allowed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateRandomOpenAccountActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must open one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: cannot open any account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateRandomCloseEmptyAccountActionInjected = true

	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "must close one account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[4]: cannot close any account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateRandomCloseDormantAccountActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "too many close dormant account actions")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 9: Unnecessary fee payments
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFeePaid = true

	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_ReceivePaymentsPublicly_RemoteSend_HappyPath(t *testing.T) {
	for _, simulateDesktopExperience := range []bool{false, true} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		receivingPhone.conf.simulateReceivingFromDesktop = simulateDesktopExperience

		giftCardAccount1 := testutil.NewRandomAccount(t)
		giftCardAccount2 := testutil.NewRandomAccount(t)
		giftCardAccount3 := testutil.NewRandomAccount(t)
		sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount1).requireSuccess(t)
		sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount2).requireSuccess(t)
		sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount3).requireSuccess(t)

		// Claimed by a different user
		submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount1, false)
		submitIntentCall.requireSuccess(t)
		server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, receivingPhone, nil)

		// Claimed by the issuer
		submitIntentCall = sendingPhone.receive42KinFromGiftCard(t, giftCardAccount2, false)
		submitIntentCall.requireSuccess(t)
		server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)

		// Voided by the issuer
		submitIntentCall = sendingPhone.receive42KinFromGiftCard(t, giftCardAccount3, true)
		submitIntentCall.requireSuccess(t)
		server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, sendingPhone, nil)
	}
}

func TestSubmitIntent_ReceivePaymentsPublicly_RemoteSend_ClaimGiftCardTwice(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)
	receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false).requireSuccess(t)

	// Simulate a race where an external deposit is made before claiming the gift
	// card on the blockchain and we haven't updated the original action's balance.
	// This isn't possible today until Geyser is explicitly hooked up to gift cards,
	// but we're being precautious to not miss any edge cases.
	server.fundAccount(t, getTimelockVault(t, giftCardAccount), kin.ToQuarks(42))

	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertStaleStateResponse(t, "gift card balance has already been claimed")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_RemoteSend_ExpiredGiftCard(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	server.simulateExpiredGiftCard(t, giftCardAccount)

	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertStaleStateResponse(t, "gift card is expired")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_RemoteSend_NonIssuerAttepmtsToVoid(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	server.simulateExpiredGiftCard(t, giftCardAccount)

	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, true)
	submitIntentCall.assertInvalidIntentResponse(t, "only the issuer can void the gift card")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_RemoteSend_ClaimInvalidGiftCardBalance(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateClaimingTooLittleFromGiftCard = true
	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive entire gift card balance of 4200000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateClaimingTooMuchFromGiftCard = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive entire gift card balance of 4200000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_AntispamGuard(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 10; i++ {
		giftCardAccount := testutil.NewRandomAccount(t)
		phone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

		submitIntentCall := phone.receive42KinFromGiftCard(t, giftCardAccount, false)
		if submitIntentCall.isError(t) {
			submitIntentCall.assertDeniedResponse(t, "too many payments")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

			return
		}

		phone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)
	}

	assert.Fail(t, "antispam guard not triggered")
}

func TestSubmitIntent_ReceivePaymentsPublicly_Validation_ManagedByCode(t *testing.T) {
	// User account
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		for _, accountType := range accountTypesToOpen {
			server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
			defer cleanup()

			server.generateAvailableNonces(t, 100)

			sendingPhone.openAccounts(t).requireSuccess(t)
			receivingPhone.openAccounts(t).requireSuccess(t)

			giftCardAccount := testutil.NewRandomAccount(t)
			sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

			server.simulateTimelockAccountInState(t, receivingPhone.getTimelockVault(t, accountType, 0), state)

			submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		}
	}

	// Gift card account
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		giftCardAccount := testutil.NewRandomAccount(t)
		sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

		server.simulateTimelockAccountInState(t, getTimelockVault(t, giftCardAccount), state)

		submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
		if state == timelock_token_v1.StateClosed {
			submitIntentCall.assertStaleStateResponse(t, "gift card balance has already been claimed")
		} else {
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
		}
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	}
}

func TestSubmitIntent_ReceivePaymentsPublicly_Validation_Balances(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivingTooMuch = true
	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: insufficient balance to perform action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_Validation_OnlyRemoteSendSupported(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFlippingRemoteSendFlag = true
	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReceivePaymentsPublicly_Validation_Actions(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	// So we (incorrectly) can use the temp accounts in some tests
	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0), kin.ToQuarks(42))
	server.fundAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, 0), kin.ToQuarks(42))

	giftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount).requireSuccess(t)

	//
	// Part 1: Validate quantity of funds being received
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivingTooLittle = true
	submitIntentCall := receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: must receive 4200000 quarks from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	// todo: Can't really test the too much case because we hit balance checks
	//       and enforce full balance is received prior to the check
	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivingTooMuch = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: insufficient balance to perform action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Validate source account
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateNotReceivingFromSource = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "must receive payments from source account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingCurrentTempIncomingAccountAsSource = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "source is not a remote send gift card")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingPrimaryAccountAsSource = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "source is not a remote send gift card")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Validate destination account
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateNotSendingToDestination = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: must send payment to latest temp incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingCurrentTempOutgoingAccountAsDestination = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: must send payment to latest temp incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateUsingPrimaryAccountAsDestination = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: must send payment to latest temp incoming account")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 5: Validate how funds are transferred between source and destination
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivePaymentsPubliclyWithoutWithdraw = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: transfer must be a public withdraw")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateReceivePaymentsPubliclyPrivately = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: transfer must be a public withdraw")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 6: Validate accounts used in money movement actions
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateInvalidTokenAccountForNoPrivacyWithdrawAction = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 7: Inject random actions that don't belong
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulatePrivacyUpgradeActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateRandomOpenAccountActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateRandomCloseDormantAccountActionInjected = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 8: Unnecessary fee payments
	//

	receivingPhone.resetConfig()
	receivingPhone.conf.simulateFeePaid = true
	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_UpgradePrivacy_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)
	server.simulateTreasuryPayments(t)

	assert.False(t, sendingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))
	assert.False(t, receivingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))

	sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)
	server.simulateTreasuryPayments(t)

	for _, phone := range []phoneTestEnv{sendingPhone, receivingPhone} {
		require.True(t, phone.checkWithServerForAnyOtherPrivacyUpgrades(t))

		submitIntentCall := phone.upgradeOneTxnToPermanentPrivacy(t, true)
		submitIntentCall.requireSuccess(t)
		server.assertPrivacyUpgraded(t, submitIntentCall.intentId, submitIntentCall.protoActions)
	}
}

func TestSubmitIntent_EstablishRelationship_Merchant_HappyPath(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	domain := "getcode.com"
	submitIntentCall := phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, phone, nil)
	server.assertFirstRelationshipAccountRecordsSaved(t, phone, domain)
}

func TestSubmitIntent_EstablishRelationship_ExistingRelationship(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 3; i++ {
		phone.resetConfig()

		domain := fmt.Sprintf("app%d.com", i)

		submitIntentCall := phone.establishRelationshipWithMerchant(t, domain)
		submitIntentCall.requireSuccess(t)
		server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, phone, nil)

		// No-op
		submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
		submitIntentCall.requireSuccess(t)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		// Derived a different account after the fact
		phone.conf.simulateDerivingDifferentRelationshipAccount = true
		submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
		submitIntentCall.assertStaleStateResponse(t, "existing relationship account exists with authority")
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		server.assertFirstRelationshipAccountRecordsSaved(t, phone, domain)
	}
}

func TestSubmitIntent_EstablishRelationship_UserAccountsNotOpened(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	domain := "getcode.com"
	submitIntentCall := phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertDeniedResponse(t, "open accounts intent not submitted")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)
}

func TestSubmitIntent_EstablishRelationship_Validation_Domain(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	for _, invalidDomain := range []string{
		"app.getcode.com",
		"localhost",
	} {
		submitIntentCall := phone.establishRelationshipWithMerchant(t, invalidDomain)
		submitIntentCall.assertInvalidIntentResponse(t, "domain is not an ascii base domain")
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
		server.assertNoRelationshipAccountRecordsSaved(t, phone, invalidDomain)
	}

	for _, invalidDomain := range []string{
		"bücher.com",
		strings.Repeat("1", 255) + ".com",
	} {
		submitIntentCall := phone.establishRelationshipWithMerchant(t, invalidDomain)
		submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
		server.assertNoRelationshipAccountRecordsSaved(t, phone, invalidDomain)
	}
}

func TestSubmitIntent_EstablishRelationship_AntispamGuard(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		enableAntispamChecks: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("app%d.com", i)
		submitIntentCall := phone.establishRelationshipWithMerchant(t, domain)
		if submitIntentCall.isError(t) {
			submitIntentCall.assertDeniedResponse(t, "too many new relationships")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
			server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

			return
		}
	}

	assert.Fail(t, "antispam guard not triggered")
}

func TestSubmitIntent_EstablishRelationship_Validation_Actions(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	domain := "getcode.com"

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	//
	// Part 1: Opening incorrect number of accounts
	//

	phone.resetConfig()
	phone.conf.simulateOpeningTooFewAccounts = true
	submitIntentCall := phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateOpeningTooManyAccounts = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	//
	// Part 2: Validation for action to open relationship account
	//

	phone.resetConfig()
	phone.conf.simulateOpenNonPrimaryAccountActionReplaced = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: expected an open account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateInvalidAccountTypeForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: account type must be RELATIONSHIP")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateInvalidTokenAccountForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: token must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateInvalidIndexForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: index must be 0")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateOwnerIsAuthorityForOpenNonPrimaryAccountAction = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertStaleStateResponse(t, "actions[0]: account is already opened")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	//
	// Part 3: Inject random actions that don't belong
	//

	phone.resetConfig()
	phone.conf.simulatePrivacyUpgradeActionInjected = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateRandomOpenAccountActionInjected = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateRandomCloseDormantAccountActionInjected = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)

	phone.resetConfig()
	phone.conf.simulateFeePaid = true
	submitIntentCall = phone.establishRelationshipWithMerchant(t, domain)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't require a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	server.assertNoRelationshipAccountRecordsSaved(t, phone, domain)
}

func TestSubmitIntent_UpgradePrivacy_Validation_Upgradeability(t *testing.T) {
	for _, tc := range []struct {
		scenario           func(*testing.T, serverTestEnv)
		errorMessage       string
		isStaleClientState bool
	}{
		{
			scenario:           func(*testing.T, serverTestEnv) {},
			errorMessage:       "must wait for next block before attempting privacy upgrade",
			isStaleClientState: false,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
			},
			errorMessage:       "must wait for next block before attempting privacy upgrade",
			isStaleClientState: false,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
				s.simulateAllTemporaryPrivateTransfersSubmitted(t)
			},
			errorMessage:       "opportunity to upgrade the private transaction was missed",
			isStaleClientState: false,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
				s.simulateAllCommitmentsUpgraded(t)
			},
			errorMessage:       "private transaction has already been upgraded",
			isStaleClientState: true,
		},
	} {
		server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		sendingPhone.openAccounts(t).requireSuccess(t)
		sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)

		tc.scenario(t, server)

		submitIntentCall := sendingPhone.upgradeOneTxnToPermanentPrivacy(t, false)
		if tc.isStaleClientState {
			submitIntentCall.assertStaleStateResponse(t, tc.errorMessage)
		} else {
			submitIntentCall.assertInvalidIntentResponse(t, tc.errorMessage)
		}
		server.assertNoPrivacyUpgrades(t, submitIntentCall.intentId, submitIntentCall.protoActions)
	}
}

func TestSubmitIntent_UpgradePrivacy_Validation_Actions(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)
	server.simulateTreasuryPayments(t)

	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)
	server.simulateTreasuryPayments(t)

	assert.True(t, sendingPhone.checkWithServerForAnyOtherPrivacyUpgrades(t))

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateDoublePrivacyUpgradeInSameRequest = true
	submitIntentCall := sendingPhone.upgradeOneTxnToPermanentPrivacy(t, true)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[1]: duplicate upgrade action detected")
	server.assertNoPrivacyUpgrades(t, submitIntentCall.intentId, submitIntentCall.protoActions)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulateInvalidActionDuringPrivacyUpgrade = true
	submitIntentCall = sendingPhone.upgradeOneTxnToPermanentPrivacy(t, true)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[1]: all actions must be to upgrade private transactions")
	server.assertNoPrivacyUpgrades(t, submitIntentCall.intentId, submitIntentCall.protoActions)
}

func TestSubmitIntent_SubmitIntentDisabled(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		disableSubmitIntent: true,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.Unavailable)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_ReuseIntentId(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	phone.conf.simulateReusingIntentId = true
	submitIntentCall := phone.deposit777KinIntoOrganizer(t)
	submitIntentCall.assertStaleStateResponse(t, "intent already exists")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_UpgradeWithNonExistantIntent(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)
	phone.deposit777KinIntoOrganizer(t).requireSuccess(t)

	phone.conf.simulateUpgradeToNonExistantIntentId = true
	submitIntentCall := phone.upgradeOneTxnToPermanentPrivacy(t, false)
	submitIntentCall.assertInvalidIntentResponse(t, "intent doesn't exists")
}

func TestSubmitIntent_NoAvailableNonces(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 0)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.Unavailable)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_NotPhoneVerified(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Resetting phone to a state where there is no phone verification
	phone.reset(t)

	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertDeniedResponse(t, "not phone verified")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_UnauthenticatedAccess(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	phone.conf.simulateInvalidSubmitIntentRequestSignature = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.Unauthenticated)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidOpenAccountSignature = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.Unauthenticated)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_InvalidActionId(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	phone.conf.simulateInvalidActionId = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.InvalidArgument)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_InvalidOpenAccountOwner(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	phone.resetConfig()
	phone.conf.simulateInvalidOpenAccountOwner = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: owner must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateOpeningGiftCardWithWrongOwner = true
	submitIntentCall = phone.send42KinToGiftCardAccount(t, testutil.NewRandomAccount(t))
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: owner must be")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_InvalidSignatureValueSubmitted(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.conf.simulateInvalidSignatureValueSubmitted = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertInvalidSignatureValueResponse(t)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_InvalidNumberOfSignaturesSubmitted(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.conf.simulateTooManySubmittedSignatures = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertSignatureErrorResponse(t, "too many signatures provided")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateTooFewSubmittedSignatures = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertSignatureErrorResponse(t, "at least one signature is missing")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_TimeBoundedRequestSend(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{
		submitIntentReceiveTimeout: 100 * time.Millisecond,
	})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.resetConfig()
	phone.conf.simulateDelayForSubmittingActions = true
	submitIntentCall := phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.DeadlineExceeded)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateDelayForSubmittingSignatures = true
	submitIntentCall = phone.openAccounts(t)
	submitIntentCall.assertGrpcError(t, codes.DeadlineExceeded)
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

}

func TestSubmitIntent_TreasuryPoolUsage(t *testing.T) {
	for _, percentUsed := range []float64{1.00, 0.99, 0.98} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		server.simulatePercentTreasuryFundsUsed(t, server.treasuryPoolByBucket[kin.ToQuarks(1)].Address, percentUsed)

		sendingPhone.openAccounts(t).requireSuccess(t)
		receivingPhone.openAccounts(t).requireSuccess(t)

		submitIntentCall := sendingPhone.send42KinToCodeUser(t, receivingPhone)
		submitIntentCall.assertGrpcError(t, codes.Unavailable)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
		submitIntentCall.assertGrpcError(t, codes.Unavailable)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
		submitIntentCall.assertGrpcError(t, codes.Unavailable)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

		submitIntentCall = sendingPhone.deposit777KinIntoOrganizer(t)
		submitIntentCall.assertGrpcError(t, codes.Unavailable)
		server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
	}
}

func TestGetPrivacyUpgradeStatus_HappyPath(t *testing.T) {
	for _, tc := range []struct {
		scenario       func(*testing.T, serverTestEnv)
		expectedResult transactionpb.GetPrivacyUpgradeStatusResponse_Result
		expectedStatus transactionpb.GetPrivacyUpgradeStatusResponse_Status
	}{
		{
			scenario:       func(*testing.T, serverTestEnv) {},
			expectedResult: transactionpb.GetPrivacyUpgradeStatusResponse_OK,
			expectedStatus: transactionpb.GetPrivacyUpgradeStatusResponse_WAITING_FOR_NEXT_BLOCK,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
			},
			expectedResult: transactionpb.GetPrivacyUpgradeStatusResponse_OK,
			expectedStatus: transactionpb.GetPrivacyUpgradeStatusResponse_WAITING_FOR_NEXT_BLOCK,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
				s.simulateAllTemporaryPrivateTransfersSubmitted(t)
			},
			expectedResult: transactionpb.GetPrivacyUpgradeStatusResponse_OK,
			expectedStatus: transactionpb.GetPrivacyUpgradeStatusResponse_TEMPORARY_TRANSACTION_FINALIZED,
		},
		{
			scenario: func(t *testing.T, s serverTestEnv) {
				s.simulateTreasuryPayments(t)
				s.simulateAllCommitmentsUpgraded(t)
			},
			expectedResult: transactionpb.GetPrivacyUpgradeStatusResponse_OK,
			expectedStatus: transactionpb.GetPrivacyUpgradeStatusResponse_ALREADY_UPGRADED,
		},
	} {
		server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		sendingPhone.openAccounts(t).requireSuccess(t)
		submitIntentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
		submitIntentCall.requireSuccess(t)

		tc.scenario(t, server)

		resp, err := sendingPhone.getPrivacyUpgradeStatus(t, submitIntentCall.intentId, 0)
		require.NoError(t, err)
		assert.Equal(t, tc.expectedResult, resp.Result)
		assert.Equal(t, tc.expectedStatus, resp.Status)
	}
}

func TestGetPrivacyUpgradeStatus_InvalidParameters(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	resp, err := sendingPhone.getPrivacyUpgradeStatus(t, testutil.NewRandomAccount(t).PublicKey().ToBase58(), 0)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrivacyUpgradeStatusResponse_ACTION_NOT_FOUND, resp.Result)

	submitIntentCall := sendingPhone.openAccounts(t)
	submitIntentCall.requireSuccess(t)

	resp, err = sendingPhone.getPrivacyUpgradeStatus(t, submitIntentCall.intentId, 0)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrivacyUpgradeStatusResponse_INVALID_ACTION, resp.Result)

	resp, err = sendingPhone.getPrivacyUpgradeStatus(t, submitIntentCall.intentId, math.MaxUint32)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrivacyUpgradeStatusResponse_ACTION_NOT_FOUND, resp.Result)
}

func TestGetPrioritizedIntentsForPrivacyUpgrade_HappyPath(t *testing.T) {
	server, sendingPhone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)

	// Make the first payment
	firstPaymentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	firstPaymentCall.requireSuccess(t)

	// There are no upgradeable intents, since the treasury hasn't paid anything out
	resp, err := sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND, resp.Result)

	// Simulate treasury payments
	server.simulateTreasuryPayments(t)

	// There are no upgradeable intents, since a proof isn't possible within the same recent root
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND, resp.Result)

	// Make the second payment
	secondPaymentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	secondPaymentCall.requireSuccess(t)

	// There are no upgradeable intents, since the treasury hasn't paid
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND, resp.Result)

	// Simulate treasury payments
	server.simulateTreasuryPayments(t)

	// The actions for the first payment are now upgradeable
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)
	require.Len(t, resp.Items, 1)

	upgradeableIntent := resp.Items[0]
	assert.Equal(t, firstPaymentCall.intentId, base58.Encode(upgradeableIntent.Id.Value))
	assert.Len(t, upgradeableIntent.Actions, 4)
	for i, expectedActionId := range []uint32{0, 1, 2, 4} {
		assert.EqualValues(t, expectedActionId, upgradeableIntent.Actions[i].ActionId)
	}
	sendingPhone.validateUpgradeableIntents(t, resp.Items)

	// Make a third payment
	thirdPaymentCall := sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	thirdPaymentCall.requireSuccess(t)
	server.simulateTreasuryPayments(t)

	// The actions for the first two payments are now upgradeable
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)
	require.Len(t, resp.Items, 2)

	firstUpgradeableIntent := resp.Items[0]
	secondUpgradeableIntent := resp.Items[1]
	assert.NotEqual(t, firstUpgradeableIntent.Id.Value, secondUpgradeableIntent.Id.Value)
	for _, upgradeableIntent := range []*transactionpb.UpgradeableIntent{firstUpgradeableIntent, secondUpgradeableIntent} {
		switch base58.Encode(upgradeableIntent.Id.Value) {
		case firstPaymentCall.intentId, secondPaymentCall.intentId:
		default:
			assert.Fail(t, "unexpected intent id")
		}

		require.Len(t, upgradeableIntent.Actions, 4)
		for i, expectedActionId := range []uint32{0, 1, 2, 4} {
			assert.EqualValues(t, expectedActionId, upgradeableIntent.Actions[i].ActionId)
		}

		sendingPhone.validateUpgradeableIntents(t, resp.Items)
	}

	// Upgrade one transaction from the first payment
	sendingPhone.upgradeOneTxnToPermanentPrivacy(t, true).requireSuccess(t)

	// That action no longer appears as upgradeable
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)
	require.Len(t, resp.Items, 2)

	firstUpgradeableIntent = resp.Items[0]
	secondUpgradeableIntent = resp.Items[1]
	assert.NotEqual(t, firstUpgradeableIntent.Id.Value, secondUpgradeableIntent.Id.Value)
	for _, upgradeableIntent := range []*transactionpb.UpgradeableIntent{firstUpgradeableIntent, secondUpgradeableIntent} {
		var expectedUpgradeableActions []uint32
		switch base58.Encode(upgradeableIntent.Id.Value) {
		case firstPaymentCall.intentId:
			expectedUpgradeableActions = []uint32{1, 2, 4}
		case secondPaymentCall.intentId:
			expectedUpgradeableActions = []uint32{0, 1, 2, 4}
		default:
			assert.Fail(t, "unexpected intent id")
		}

		require.Len(t, upgradeableIntent.Actions, len(expectedUpgradeableActions))
		for i, expectedActionId := range expectedUpgradeableActions {
			assert.EqualValues(t, expectedActionId, upgradeableIntent.Actions[i].ActionId)
		}

		sendingPhone.validateUpgradeableIntents(t, resp.Items)
	}

	// Upgrade the remaining transactions for the first payment
	for i := 0; i < 3; i++ {
		sendingPhone.upgradeOneTxnToPermanentPrivacy(t, true).requireSuccess(t)
	}

	// Only actions from the second payment is shown as upgradeable
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK, resp.Result)
	require.Len(t, resp.Items, 1)

	upgradeableIntent = resp.Items[0]
	assert.Equal(t, secondPaymentCall.intentId, base58.Encode(upgradeableIntent.Id.Value))
	require.Len(t, upgradeableIntent.Actions, 4)
	for i, expectedActionId := range []uint32{0, 1, 2, 4} {
		assert.EqualValues(t, expectedActionId, upgradeableIntent.Actions[i].ActionId)
	}

	// Simulate cashing in the temporary privacy transfers
	server.simulateAllTemporaryPrivateTransfersSubmitted(t)

	// There are no upgradeable intents
	resp, err = sendingPhone.getPrioritizedIntentsForPrivacyUpgrade(t)
	require.NoError(t, err)
	assert.Equal(t, transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND, resp.Result)
}

func TestGetPrioritizedIntentsForPrivacyUpgrade_UnauthenticatedAccess(t *testing.T) {
	_, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	phone.conf.simulateInvalidGetPrioritizedIntentsForPrivacyUpgradeSignature = true

	_, err := phone.getPrioritizedIntentsForPrivacyUpgrade(t)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func TestSubmitIntent_MigrateToPrivacy2022_HappyPath_EmptyAccount(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	submitIntentCall := phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, phone, nil)
}

func TestSubmitIntent_MigrateToPrivacy2022_HappyPath_PositiveBalance(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	amountToMigrate := kin.ToQuarks(23)

	legacyTimelockVault, err := phone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
	require.NoError(t, err)
	server.fundAccount(t, legacyTimelockVault, amountToMigrate)

	submitIntentCall := phone.migrateToPrivacy2022(t, amountToMigrate)
	submitIntentCall.requireSuccess(t)
	server.assertIntentSubmitted(t, submitIntentCall.intentId, submitIntentCall.protoMetadata, submitIntentCall.protoActions, phone, nil)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_PrivacyAccountsNotOpened(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	submitIntentCall := phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertStaleStateResponse(t, "must submit open accounts intent")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_AlreadyMigrated(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)
	phone.migrateToPrivacy2022(t, 0).requireSuccess(t)

	submitIntentCall := phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertStaleStateResponse(t, "already submitted intent to migrate to privacy")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_NoMigrationPath(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	// Reset phone to a state where they are verified, but don't have a legacy account
	phone.reset(t)
	server.simulatePhoneVerifyingUser(t, phone)

	submitIntentCall := phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertStaleStateResponse(t, "no account to migrate")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_Balances(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	phone.migrateToPrivacy2022(t, 1).assertInvalidIntentResponse(t, "must migrate 0 quarks")

	amountToMigrate := kin.ToQuarks(23)

	legacyTimelockVault, err := phone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
	require.NoError(t, err)
	server.fundAccount(t, legacyTimelockVault, amountToMigrate)

	submitIntentCall := phone.migrateToPrivacy2022(t, amountToMigrate-1)
	submitIntentCall.assertInvalidIntentResponse(t, "must migrate 2300000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = phone.migrateToPrivacy2022(t, amountToMigrate+1)
	submitIntentCall.assertInvalidIntentResponse(t, "must migrate 2300000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, "must migrate 2300000 quarks")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_ManagedByCode(t *testing.T) {
	for _, state := range []timelock_token_v1.TimelockState{
		timelock_token_v1.StateUnlocked,
		timelock_token_v1.StateWaitingForTimeout,
		timelock_token_v1.StateClosed,
	} {
		for _, accountType := range []commonpb.AccountType{
			commonpb.AccountType_PRIMARY,
			commonpb.AccountType_LEGACY_PRIMARY_2022,
		} {
			server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
			defer cleanup()

			server.generateAvailableNonces(t, 100)

			phone.openAccounts(t).requireSuccess(t)

			var tokenAccount *common.Account
			if accountType == commonpb.AccountType_PRIMARY {
				tokenAccount = phone.getTimelockVault(t, accountType, 0)
			} else {
				legacyTimelockVault, err := phone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
				require.NoError(t, err)
				tokenAccount = legacyTimelockVault
			}
			server.simulateTimelockAccountInState(t, tokenAccount, state)

			submitIntentCall := phone.migrateToPrivacy2022(t, 0)
			submitIntentCall.assertDeniedResponse(t, "at least one account is no longer managed by code")
			server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
		}
	}
}

func TestSubmitIntent_PaymentRequest_Validation(t *testing.T) {
	server, phone1, phone2, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	giftCardAccount := testutil.NewRandomAccount(t)
	phone2.openAccounts(t).requireSuccess(t)
	phone2.send42KinToGiftCardAccount(t, giftCardAccount)

	//
	// Part 1: Using payment request intent ID in an invalid intent type
	//

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall := phone1.openAccounts(t)
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall = phone1.receive42KinPrivatelyIntoOrganizer(t)
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall = phone1.migrateToPrivacy2022(t, kin.ToQuarks(42))
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall = phone1.receive42KinFromGiftCard(t, giftCardAccount, false)
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall = phone1.publiclyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	submitIntentCall = phone1.establishRelationshipWithMerchant(t, "getcode.com")
	submitIntentCall.assertDeniedResponse(t, "intent id is reserved for a request")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 2: Client deviates from the expected payment request
	//

	phone1.resetConfig()
	phone1.openAccounts(t).requireSuccess(t)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateInvalidPaymentRequestDestination = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "payment has a request to destination")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateInvalidPaymentRequestExchangeCurrency = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "payment has a request for aed currency")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateInvalidPaymentRequestNativeAmount = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "payment has a request for 123.01 native amount")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Client doesn't adhere to fee parameters
	//

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateNoFeesPaid = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "intent requires a fee payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateMultipleFeePayments = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "fee payment must be done in a single action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateSmallFee = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "fee payment must be $0.01 USD")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateLargeFee = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "fee payment must be $0.01 USD")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	//
	// Part 3: Client attempts to pay a login request
	//
	phone1.resetConfig()
	phone1.conf.simulatePaymentRequest = true
	phone1.conf.simulateLoginRequest = true
	submitIntentCall = phone1.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.assertInvalidIntentResponse(t, "request doesn't require payment")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestSubmitIntent_MigrateToPrivacy2022_Validation_Actions(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	phone.openAccounts(t).requireSuccess(t)

	legacyTimelockVault, err := phone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
	require.NoError(t, err)

	phone.resetConfig()
	phone.conf.simulateClosingWrongAccount = true
	submitIntentCall := phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, "actions[0]: account type must be LEGACY_PRIMARY_2022")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidTokenAccountForCloseEmptyAccountAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[0]: must migrate from account %s", legacyTimelockVault.PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateInvalidAuthorityForCloseEmptyAccountAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[0]: authority must be %s", phone.parentAccount.PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateRandomCloseEmptyAccountActionInjected = true
	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, "expected 1 action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateMigratingUsingOppositeAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, "expected a close empty account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateMigratingUsingWrongAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, 0)
	submitIntentCall.assertInvalidIntentResponse(t, "expected a close empty account action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	server.fundAccount(t, legacyTimelockVault, kin.ToQuarks(1))

	phone.resetConfig()
	phone.conf.simulateMigratingFundsToWrongAccount = true
	submitIntentCall = phone.migrateToPrivacy2022(t, kin.ToQuarks(1))
	submitIntentCall.assertInvalidIntentResponse(t, fmt.Sprintf("actions[0]: must migrate funds to account %s", phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBase58()))
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulatingMigratingWithWrongAmountInAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, kin.ToQuarks(1))
	submitIntentCall.assertInvalidIntentResponse(t, "quark amount must match intent metadata")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateMigratingUsingOppositeAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, kin.ToQuarks(1))
	submitIntentCall.assertInvalidIntentResponse(t, "expected a no privacy withdraw action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)

	phone.resetConfig()
	phone.conf.simulateMigratingUsingWrongAction = true
	submitIntentCall = phone.migrateToPrivacy2022(t, kin.ToQuarks(1))
	submitIntentCall.assertInvalidIntentResponse(t, "expected a no privacy withdraw action")
	server.assertIntentNotSubmitted(t, submitIntentCall.intentId)
}

func TestGetIntentMetadata_HappyPath(t *testing.T) {
	for _, signAsOwner := range []bool{true, false} {
		server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
		defer cleanup()

		server.generateAvailableNonces(t, 100)

		resp := sendingPhone.getIntentMetadata(t, testutil.NewRandomAccount(t), signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		receivingPhone.openAccounts(t).requireSuccess(t)

		// OpenAccounts intent

		submitIntentCall := sendingPhone.openAccounts(t)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetOpenAccounts())

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		// MigrateToPrivacy2022 intent

		amountToMigrate := kin.ToQuarks(1)
		legacyTimelockVault, err := sendingPhone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
		require.NoError(t, err)
		server.fundAccount(t, legacyTimelockVault, amountToMigrate)

		submitIntentCall = sendingPhone.migrateToPrivacy2022(t, amountToMigrate)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetMigrateToPrivacy_2022())
		assert.Equal(t, amountToMigrate, resp.Metadata.GetMigrateToPrivacy_2022().GetQuarks())

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		// SendPrivatePayment intent

		submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPrivatePayment())
		assert.EqualValues(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBytes(), resp.Metadata.GetSendPrivatePayment().Destination.Value)
		assert.EqualValues(t, currency.USD, resp.Metadata.GetSendPrivatePayment().ExchangeData.Currency)
		assert.Equal(t, 0.1, resp.Metadata.GetSendPrivatePayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 4.2, resp.Metadata.GetSendPrivatePayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetSendPrivatePayment().ExchangeData.Quarks)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsWithdrawal)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsRemoteSend)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPrivatePayment())
		assert.EqualValues(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBytes(), resp.Metadata.GetSendPrivatePayment().Destination.Value)
		assert.EqualValues(t, currency.USD, resp.Metadata.GetSendPrivatePayment().ExchangeData.Currency)
		assert.Equal(t, 0.1, resp.Metadata.GetSendPrivatePayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 4.2, resp.Metadata.GetSendPrivatePayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetSendPrivatePayment().ExchangeData.Quarks)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsWithdrawal)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsRemoteSend)

		submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPrivatePayment())
		assert.EqualValues(t, currency.KIN, resp.Metadata.GetSendPrivatePayment().ExchangeData.Currency)
		assert.Equal(t, 1.0, resp.Metadata.GetSendPrivatePayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 123.0, resp.Metadata.GetSendPrivatePayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(123), resp.Metadata.GetSendPrivatePayment().ExchangeData.Quarks)
		assert.True(t, resp.Metadata.GetSendPrivatePayment().IsWithdrawal)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsRemoteSend)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		giftCardAccount := testutil.NewRandomAccount(t) // Explicitly a random account
		submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, giftCardAccount)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPrivatePayment())
		assert.EqualValues(t, getTimelockVault(t, giftCardAccount).PublicKey().ToBytes(), resp.Metadata.GetSendPrivatePayment().Destination.Value)
		assert.EqualValues(t, currency.CAD, resp.Metadata.GetSendPrivatePayment().ExchangeData.Currency)
		assert.Equal(t, 0.05, resp.Metadata.GetSendPrivatePayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 2.1, resp.Metadata.GetSendPrivatePayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetSendPrivatePayment().ExchangeData.Quarks)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsWithdrawal)
		assert.True(t, resp.Metadata.GetSendPrivatePayment().IsRemoteSend)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		voidedGiftCardAccount := testutil.NewRandomAccount(t) // Explicitly a random account
		submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, voidedGiftCardAccount)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPrivatePayment())
		assert.EqualValues(t, getTimelockVault(t, voidedGiftCardAccount).PublicKey().ToBytes(), resp.Metadata.GetSendPrivatePayment().Destination.Value)
		assert.EqualValues(t, currency.CAD, resp.Metadata.GetSendPrivatePayment().ExchangeData.Currency)
		assert.Equal(t, 0.05, resp.Metadata.GetSendPrivatePayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 2.1, resp.Metadata.GetSendPrivatePayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetSendPrivatePayment().ExchangeData.Quarks)
		assert.False(t, resp.Metadata.GetSendPrivatePayment().IsWithdrawal)
		assert.True(t, resp.Metadata.GetSendPrivatePayment().IsRemoteSend)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		// ReceivePaymentsPrivately intent

		submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPrivately())
		assert.EqualValues(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0).PublicKey().ToBytes(), resp.Metadata.GetReceivePaymentsPrivately().Source.Value)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetReceivePaymentsPrivately().Quarks)
		assert.False(t, resp.Metadata.GetReceivePaymentsPrivately().IsDeposit)

		submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPrivately())
		assert.EqualValues(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBytes(), resp.Metadata.GetReceivePaymentsPrivately().Source.Value)
		assert.Equal(t, kin.ToQuarks(777), resp.Metadata.GetReceivePaymentsPrivately().Quarks)
		assert.True(t, resp.Metadata.GetReceivePaymentsPrivately().IsDeposit)

		// SendPublicPayment intent

		submitIntentCall = sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPublicPayment())
		assert.EqualValues(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0).PublicKey().ToBytes(), resp.Metadata.GetSendPublicPayment().Destination.Value)
		assert.EqualValues(t, currency.USD, resp.Metadata.GetSendPublicPayment().ExchangeData.Currency)
		assert.Equal(t, 0.1, resp.Metadata.GetSendPublicPayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 77.7, resp.Metadata.GetSendPublicPayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(777), resp.Metadata.GetSendPublicPayment().ExchangeData.Quarks)
		assert.True(t, resp.Metadata.GetSendPublicPayment().IsWithdrawal)

		submitIntentCall = sendingPhone.publiclyWithdraw123KinToExternalWallet(t)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetSendPublicPayment())
		assert.EqualValues(t, currency.KIN, resp.Metadata.GetSendPublicPayment().ExchangeData.Currency)
		assert.Equal(t, 1.0, resp.Metadata.GetSendPublicPayment().ExchangeData.ExchangeRate)
		assert.Equal(t, 123.0, resp.Metadata.GetSendPublicPayment().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(123), resp.Metadata.GetSendPublicPayment().ExchangeData.Quarks)
		assert.True(t, resp.Metadata.GetSendPublicPayment().IsWithdrawal)

		// ReceivePaymentsPublicly intent

		submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAccount, false)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPublicly())
		assert.EqualValues(t, getTimelockVault(t, giftCardAccount).PublicKey().ToBytes(), resp.Metadata.GetReceivePaymentsPublicly().Source.Value)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetReceivePaymentsPublicly().Quarks)
		assert.True(t, resp.Metadata.GetReceivePaymentsPublicly().IsRemoteSend)
		assert.False(t, resp.Metadata.GetReceivePaymentsPublicly().IsIssuerVoidingGiftCard)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData)
		assert.Equal(t, "cad", resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.Currency)
		assert.Equal(t, 0.05, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.ExchangeRate)
		assert.Equal(t, 2.1, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.Quarks)

		submitIntentCall = sendingPhone.receive42KinFromGiftCard(t, voidedGiftCardAccount, true)
		submitIntentCall.requireSuccess(t)

		resp = sendingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, signAsOwner)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_OK, resp.Result)
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPublicly())
		assert.EqualValues(t, getTimelockVault(t, voidedGiftCardAccount).PublicKey().ToBytes(), resp.Metadata.GetReceivePaymentsPublicly().Source.Value)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetReceivePaymentsPublicly().Quarks)
		assert.True(t, resp.Metadata.GetReceivePaymentsPublicly().IsRemoteSend)
		assert.True(t, resp.Metadata.GetReceivePaymentsPublicly().IsIssuerVoidingGiftCard)
		require.NotNil(t, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData)
		assert.Equal(t, "cad", resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.Currency)
		assert.Equal(t, 0.05, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.ExchangeRate)
		assert.Equal(t, 2.1, resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.NativeAmount)
		assert.Equal(t, kin.ToQuarks(42), resp.Metadata.GetReceivePaymentsPublicly().ExchangeData.Quarks)

		resp = receivingPhone.getIntentMetadata(t, submitIntentCall.rendezvousKey, true)
		assert.Equal(t, transactionpb.GetIntentMetadataResponse_NOT_FOUND, resp.Result)
		assert.Nil(t, resp.Metadata)
	}
}

func TestCanWithdrawToAccount_CodeAccounts(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	legacyAccount, err := receivingPhone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
	require.NoError(t, err)

	giftCardAuthorityAccount := testutil.NewRandomAccount(t)
	giftCardVaultAccount := getTimelockVault(t, giftCardAuthorityAccount)

	sendingPhone.send42KinToGiftCardAccount(t, giftCardAuthorityAccount).requireSuccess(t)

	sendingPhone.assertCanWithdrawToAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), true, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
	sendingPhone.assertCanWithdrawToAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_INCOMING, 0), false, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
	sendingPhone.assertCanWithdrawToAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_TEMPORARY_OUTGOING, 0), false, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
	sendingPhone.assertCanWithdrawToAccount(t, receivingPhone.getTimelockVault(t, commonpb.AccountType_BUCKET_100_KIN, 0), false, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
	sendingPhone.assertCanWithdrawToAccount(t, legacyAccount, false, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
	sendingPhone.assertCanWithdrawToAccount(t, giftCardVaultAccount, false, transactionpb.CanWithdrawToAccountResponse_TokenAccount)
}

func TestGetTranscript(t *testing.T) {
	source, err := common.NewAccountFromPublicKeyString("GNVyMgwkFQvm3YLuJdEVW4xEoqDYnixVaxVYT59frGWW")
	require.NoError(t, err)

	destination, err := common.NewAccountFromPublicKeyString("Cia66LdCtvfJ6G5jjmLtNoFx5JvWr3uNv2iaFvmSS9gW")
	require.NoError(t, err)

	transcript := getTransript(
		"4roBdWPCqbuqr4YtPavfi7hTAMdH52RXMDgKhqQ4qvX6",
		1,
		source,
		destination,
		kin.ToQuarks(40),
	)
	assert.Equal(t, "438d0b389153b7d0116ac6528fa22dffa7e16499e6923f341b78c6641e418287", hex.EncodeToString(transcript))
}

// todo: Any way we can test the actual locking in a simple way? It might be easier
// when we have a test abstraction for a real distributed lock interface. This would
// also allow us to cover the other things we lock on too.
func TestSubmitIntent_AdditionalAccountsToLock(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 100)

	//
	// Opening Accounts
	//

	submitIntentCall := sendingPhone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err := server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err := NewOpenAccountsIntentHandler(server.service.conf, server.data, server.service.antispamGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)

	submitIntentCall = receivingPhone.openAccounts(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewOpenAccountsIntentHandler(server.service.conf, server.data, server.service.antispamGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)

	//
	// Direct Code->Code payment
	//

	submitIntentCall = sendingPhone.send42KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewSendPrivatePaymentIntentHandler(server.service.conf, server.data, server.service.pusher, server.service.antispamGuard, server.service.amlGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	require.NotNil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)
	assert.Equal(t, receivingPhone.parentAccount.PublicKey().ToBase58(), accountsToLock.DestinationOwner.PublicKey().ToBase58())

	submitIntentCall = receivingPhone.receive42KinFromCodeUser(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewReceivePaymentsPrivatelyIntentHandler(server.service.conf, server.data, server.service.antispamGuard, server.service.amlGuard).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)

	//
	// Withdraw/Deposit
	//

	submitIntentCall = sendingPhone.privatelyWithdraw123KinToExternalWallet(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewSendPrivatePaymentIntentHandler(server.service.conf, server.data, server.service.pusher, server.service.antispamGuard, server.service.amlGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)

	submitIntentCall = sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewSendPrivatePaymentIntentHandler(server.service.conf, server.data, server.service.pusher, server.service.antispamGuard, server.service.amlGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	require.NotNil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)
	assert.Equal(t, receivingPhone.parentAccount.PublicKey().ToBase58(), accountsToLock.DestinationOwner.PublicKey().ToBase58())

	submitIntentCall = receivingPhone.deposit777KinIntoOrganizer(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewReceivePaymentsPrivatelyIntentHandler(server.service.conf, server.data, server.service.antispamGuard, server.service.amlGuard).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)

	//
	// Remote Send
	//

	giftCardAuthorityAccount := testutil.NewRandomAccount(t)
	giftCardVaultAccount := getTimelockVault(t, giftCardAuthorityAccount)

	submitIntentCall = sendingPhone.send42KinToGiftCardAccount(t, giftCardAuthorityAccount)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewSendPrivatePaymentIntentHandler(server.service.conf, server.data, server.service.pusher, server.service.antispamGuard, server.service.amlGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	require.NotNil(t, accountsToLock.RemoteSendGiftCardVault)
	assert.Equal(t, giftCardVaultAccount.PublicKey().ToBase58(), accountsToLock.RemoteSendGiftCardVault.PublicKey().ToBase58())

	submitIntentCall = receivingPhone.receive42KinFromGiftCard(t, giftCardAuthorityAccount, false)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewReceivePaymentsPubliclyIntentHandler(server.service.conf, server.data, server.service.antispamGuard, nil).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	require.NotNil(t, accountsToLock.RemoteSendGiftCardVault)
	assert.Equal(t, giftCardVaultAccount.PublicKey().ToBase58(), accountsToLock.RemoteSendGiftCardVault.PublicKey().ToBase58())

	submitIntentCall = receivingPhone.receive42KinPrivatelyIntoOrganizer(t)
	submitIntentCall.requireSuccess(t)
	intentRecord, err = server.data.GetIntent(server.ctx, submitIntentCall.intentId)
	require.NoError(t, err)
	accountsToLock, err = NewReceivePaymentsPrivatelyIntentHandler(server.service.conf, server.data, server.service.antispamGuard, server.service.amlGuard).GetAdditionalAccountsToLock(server.ctx, intentRecord)
	require.NoError(t, err)
	assert.Nil(t, accountsToLock.DestinationOwner)
	assert.Nil(t, accountsToLock.RemoteSendGiftCardVault)
}
