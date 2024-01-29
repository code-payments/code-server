package async_sequencer

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

// todo: Still not entirely happy how temporary incoming/outgoing accounts are handled in these tests. Lots of manual and error prone input still.

var isVerbose bool

func init() {
	for _, arg := range os.Args {
		if arg == "-test.v=true" {
			isVerbose = true
		}
	}
}

func TestContextualScheduler_GenericFlow_FulfillmentHandler(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(), OpenAccountsMetadata: &intent.OpenAccountsMetadata{}}}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
	fulfillmentRecord := fulfillmentRecords[0]

	mockHandler := &mockFulfillmentHandler{
		isScheduled: true,
	}
	env.scheduler.handlersByType[fulfillmentRecord.FulfillmentType] = mockHandler
	env.assertSchedulingState(t, fulfillmentRecord, true)

	mockHandler.isScheduled = false
	env.assertSchedulingState(t, fulfillmentRecord, false)
}

func TestContextualScheduler_GenericFlow_FulfillmentState(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(), OpenAccountsMetadata: &intent.OpenAccountsMetadata{}}}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
	fulfillmentRecord := fulfillmentRecords[0]

	env.scheduler.handlersByType[fulfillmentRecord.FulfillmentType] = &mockFulfillmentHandler{
		isScheduled: true,
	}

	for _, state := range []fulfillment.State{
		fulfillment.StateUnknown,
		fulfillment.StatePending,
	} {
		fulfillmentRecord.State = state
		env.assertSchedulingState(t, fulfillmentRecord, true)
	}

	for _, state := range []fulfillment.State{
		fulfillment.StateRevoked,
		fulfillment.StateConfirmed,
		fulfillment.StateFailed,
	} {
		fulfillmentRecord.State = state
		env.assertSchedulingState(t, fulfillmentRecord, false)
	}
}

func TestContextualScheduler_GenericFlow_FulfillmentAlreadyPending(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(), OpenAccountsMetadata: &intent.OpenAccountsMetadata{}}}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
	fulfillmentRecord := fulfillmentRecords[0]

	mockHandler := &mockFulfillmentHandler{
		isScheduled: false,
	}
	env.scheduler.handlersByType[fulfillmentRecord.FulfillmentType] = mockHandler
	env.assertSchedulingState(t, fulfillmentRecord, false)

	fulfillmentRecord.State = fulfillment.StatePending
	env.assertSchedulingState(t, fulfillmentRecord, true)
}

func TestContextualScheduler_GenericFlow_ActionState(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(), OpenAccountsMetadata: &intent.OpenAccountsMetadata{}}}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
	fulfillmentRecord := fulfillmentRecords[0]

	env.scheduler.handlersByType[fulfillmentRecord.FulfillmentType] = &mockFulfillmentHandler{
		isScheduled: true,
	}
	env.assertSchedulingState(t, fulfillmentRecord, true)

	for _, state := range []action.State{
		action.StateUnknown,
		action.StatePending,
		action.StateRevoked,
		action.StateConfirmed,
		action.StateFailed,
	} {
		actionRecord, err := env.data.GetActionById(env.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		require.NoError(t, err)
		actionRecord.State = state
		require.NoError(t, env.data.UpdateAction(env.ctx, actionRecord))

		env.assertSchedulingState(t, fulfillmentRecord, state == action.StatePending || state == action.StateConfirmed)
	}
}

func TestContextualScheduler_GenericFlow_NoSignature(t *testing.T) {
	for _, supportsOnDemandTxnCreation := range []bool{true, false} {
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(), OpenAccountsMetadata: &intent.OpenAccountsMetadata{}}}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
		fulfillmentRecord := fulfillmentRecords[0]

		env.scheduler.handlersByType[fulfillmentRecord.FulfillmentType] = &mockFulfillmentHandler{
			isScheduled:                 true,
			supportsOnDemandTxnCreation: supportsOnDemandTxnCreation,
		}
		env.assertSchedulingState(t, fulfillmentRecord, true)

		fulfillmentRecord.Signature = nil
		env.assertSchedulingState(t, fulfillmentRecord, supportsOnDemandTxnCreation)
	}
}

func TestContextualScheduler_GenericFlow_AccountCircuitBreaker(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	for key := range env.scheduler.handlersByType {
		env.scheduler.handlersByType[key] = &mockFulfillmentHandler{
			isScheduled: true,
		}
	}

	alice := "alice"
	bob := "bob"
	intentRecords := []*intent.Record{
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	for _, fulfillmentRecord := range fulfillmentRecords {
		// Closing dormant accounts are unscheduled by default and ignored for this test
		if fulfillmentRecord.ActionType == action.CloseDormantAccount {
			continue
		}

		env.assertSchedulingState(t, fulfillmentRecord, true)
	}

	var fulfillmentToFail *fulfillment.Record
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == "bob-incoming-1" {
			fulfillmentToFail = fulfillmentRecord
		}
	}
	require.NotNil(t, fulfillmentToFail)

	env.simulateFailedFulfillment(t, fulfillmentToFail)
	for _, fulfillmentRecord := range fulfillmentRecords {
		// Closing dormant accounts are unscheduled by default and ignored for this test
		if fulfillmentRecord.ActionType == action.CloseDormantAccount {
			continue
		}

		if fulfillmentRecord.Intent == fulfillmentToFail.Intent {
			continue
		}

		expectedState := true
		if fulfillmentRecord.Source == "bob-incoming-1" {
			expectedState = false
		} else if fulfillmentRecord.Destination != nil && *fulfillmentRecord.Destination == "bob-incoming-1" {
			expectedState = false
		}
		env.assertSchedulingState(t, fulfillmentRecord, expectedState)
	}
}

func TestContextualScheduler_GenericFlow_IntentCircuitBreaker(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	for key := range env.scheduler.handlersByType {
		env.scheduler.handlersByType[key] = &mockFulfillmentHandler{
			isScheduled: true,
		}
	}

	alice := "alice"
	bob := "bob"
	charlie := "charlie"
	intentRecords := []*intent.Record{
		// Intent which we'll fail a fulfillment and trip the intent-level circuit
		// breaker
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: charlie, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},

		// Completely unrelated intents whose fulfillment scheduling state won't
		// be affected
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	for _, fulfillmentRecord := range fulfillmentRecords {
		// Closing dormant accounts are unscheduled by default and ignored for this test
		if fulfillmentRecord.ActionType == action.CloseDormantAccount {
			continue
		}

		env.assertSchedulingState(t, fulfillmentRecord, true)
	}

	fulfillmentToFail := fulfillmentRecords[0]
	env.simulateFailedFulfillment(t, fulfillmentToFail)
	for _, fulfillmentRecord := range fulfillmentRecords {
		// Closing dormant accounts are unscheduled by default and ignored for this test
		if fulfillmentRecord.ActionType == action.CloseDormantAccount {
			continue
		}

		env.assertSchedulingState(t, fulfillmentRecord, fulfillmentRecord.Intent != fulfillmentToFail.Intent)
	}
}

func TestContextualScheduler_GenericFlow_GlobalCircuitBreaker(t *testing.T) {
	maxFailures := 3

	env := setupSchedulerEnv(t, &testOverrides{
		maxGlobalFailedFulfillments: uint64(maxFailures),
	})
	for key := range env.scheduler.handlersByType {
		env.scheduler.handlersByType[key] = &mockFulfillmentHandler{
			isScheduled: true,
		}
	}

	alice := "alice"
	bob := "bob"
	charlie := "charlie"
	intentRecords := []*intent.Record{
		// Completely unrelated intent for which we'll fail fulfillments
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: charlie, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},

		// Fulfillments in other intents that will eventually get tripped by the
		// global circuit breaker
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	intentWithFailure := fulfillmentRecords[0].Intent
	for i := 0; i < 2*maxFailures; i++ {
		var simulatedFailure bool
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.Intent == intentWithFailure && fulfillmentRecord.State != fulfillment.StateFailed {
				simulatedFailure = true
				env.simulateFailedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		assert.True(t, simulatedFailure)

		for _, fulfillmentRecord := range fulfillmentRecords {
			// Closing dormant accounts are unscheduled by default and ignored for this test
			if fulfillmentRecord.ActionType == action.CloseDormantAccount {
				continue
			}

			if fulfillmentRecord.Intent == intentWithFailure {
				continue
			}

			env.assertSchedulingState(t, fulfillmentRecord, i <= maxFailures-1)
		}
	}
}

func TestContextualScheduler_GenericFlow_SchedulingManuallyDisabled(t *testing.T) {
	for _, configValue := range []bool{true, false} {
		env := setupSchedulerEnv(t, &testOverrides{
			disableTransactionScheduling: configValue,
		})
		for key := range env.scheduler.handlersByType {
			env.scheduler.handlersByType[key] = &mockFulfillmentHandler{
				isScheduled: true,
			}
		}

		alice := "alice"
		bob := "bob"
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		for _, fulfillmentRecord := range fulfillmentRecords {
			// Closing dormant accounts are unscheduled by default and ignored for this test
			if fulfillmentRecord.ActionType == action.CloseDormantAccount {
				continue
			}

			env.assertSchedulingState(t, fulfillmentRecord, !configValue)
		}
	}
}

func TestContextualScheduler_OnDemandUserAccountCreation(t *testing.T) {
	alice := "alice"

	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	for _, fulfillmentRecord := range fulfillmentRecords {
		var expected bool
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.InitializeLockedTimelockAccount:
			expected = fulfillmentRecord.Source == "alice-primary"
		}

		env.assertSchedulingState(t, fulfillmentRecord, expected)
	}

	env = setupSchedulerEnv(t, &testOverrides{})
	intentRecords = []*intent.Record{
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: alice, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "alice-incoming-1", Quantity: kin.ToQuarks(42)}},
	}
	fulfillmentRecords = env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	for _, fulfillmentRecord := range fulfillmentRecords {
		// Account doesn't receive funds, so the test will error out if we don't skip it.
		if fulfillmentRecord.Source == "alice-incoming-1" {
			continue
		}

		var expected bool
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.InitializeLockedTimelockAccount:
			switch fulfillmentRecord.Source {
			case "alice-primary", "alice-bucket-1-kin", "alice-bucket-10-kin":
				expected = true
			}
		}

		env.assertSchedulingState(t, fulfillmentRecord, expected)
	}
}

func TestContextualScheduler_TreasuryPayments_UnlimitedFunds(t *testing.T) {
	alice := "alice"
	bob := "bob"
	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(999)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(999)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{
		treasuryPoolFunds: kin.ToQuarks(10000000),
	})

	openedAccounts := make(map[string]struct{})
	accountOpens := filterFulfillmentsByType(fulfillmentRecords, fulfillment.InitializeLockedTimelockAccount)
	treasuryPayments := filterFulfillmentsByType(fulfillmentRecords, fulfillment.TransferWithCommitment)
	for _, treasuryPayment := range treasuryPayments {
		_, alreadyOpened := openedAccounts[*treasuryPayment.Destination]
		env.assertSchedulingState(t, treasuryPayment, alreadyOpened)

		for _, accountOpen := range accountOpens {
			if accountOpen.Source == *treasuryPayment.Destination {
				openedAccounts[*treasuryPayment.Destination] = struct{}{}
				env.simulateConfirmedFulfillment(t, accountOpen)
				break
			}
		}

		env.assertSchedulingState(t, treasuryPayment, true)
	}

	for _, treasuryPayment := range treasuryPayments {
		env.assertSchedulingState(t, treasuryPayment, true)
	}
}

func TestContextualScheduler_TreasuryPayments_LimitedFunds(t *testing.T) {
	for _, fundingLevel := range []uint64{
		kin.ToQuarks(999),  // Exactly fits a subset of payments
		kin.ToQuarks(1000), // Will always have a remaining amount
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(999)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(999)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{
			treasuryPoolFunds: fundingLevel,
		})

		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}

		scheduledFulfillments := make(map[string]struct{})
		treasuryPayments := filterFulfillmentsByType(fulfillmentRecords, fulfillment.TransferWithCommitment)
		for _, treasuryPayment := range treasuryPayments {
			isScheduled, err := env.scheduler.CanSubmitToBlockchain(env.ctx, treasuryPayment)
			require.NoError(t, err)

			if isScheduled {
				scheduledFulfillments[*treasuryPayment.Signature] = struct{}{}
			}
		}

		assert.Len(t, scheduledFulfillments, 3)
		env.assertReservedTreasuryFunds(t, kin.ToQuarks(999))

		for _, treasuryPayment := range treasuryPayments {
			_, alreadyScheduled := scheduledFulfillments[*treasuryPayment.Signature]
			env.assertSchedulingState(t, treasuryPayment, alreadyScheduled)
		}
	}
}

func TestContextualScheduler_NoPrivacyTransfer(t *testing.T) {
	simulateOpeningSourceAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOpeningDestinationAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == *transfer.Destination {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	forceSimulateDependentWithdraw := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw && *fulfillmentRecord.Destination == transfer.Source {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {},
			expected:   false,
		},
		{
			// Dependent withdraw not paid
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Destination account not opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				forceSimulateDependentWithdraw(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Source user account not opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				forceSimulateDependentWithdraw(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				forceSimulateDependentWithdraw(env, transfer, fulfillmentRecords)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-primary", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPublicPayment, InitiatorOwnerAccount: alice, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationTokenAccount: "bob-primary", Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.NoPrivacyTransferWithAuthority)
		require.Len(t, filtered, 1)
		noPrivacyTransfer := filtered[0]
		env.assertSchedulingState(t, noPrivacyTransfer, false)

		tc.simulation(env, noPrivacyTransfer, fulfillmentRecords)
		env.assertSchedulingState(t, noPrivacyTransfer, tc.expected)
	}
}

func TestContextualScheduler_NoPrivacyWithdraw(t *testing.T) {
	simulateOpeningSourceAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOpeningDestinationAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == *transfer.Destination {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOneTreasuryPayment := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateAllTreasuryPayments := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}
	}

	forceSimulateAllTreasuryPayments := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
			}
		}
	}

	forceSimulateFeePayment := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyTransferWithAuthority && fulfillmentRecord.Source == transfer.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {},
			expected:   false,
		},
		{
			// Treasury payments not confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				forceSimulateFeePayment(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Subset of treasury payments confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				simulateOneTreasuryPayment(env, transfer, fulfillmentRecords)
				forceSimulateFeePayment(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Destination account not opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				forceSimulateFeePayment(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Source user account not opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				forceSimulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				forceSimulateFeePayment(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Fee payment not confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, transfer, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				forceSimulateFeePayment(env, transfer, fulfillmentRecords)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", IsMicroPayment: true, Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.NoPrivacyWithdraw)
		require.Len(t, filtered, 1)
		noPrivacyWithdraw := filtered[0]
		env.assertSchedulingState(t, noPrivacyWithdraw, false)

		tc.simulation(env, noPrivacyWithdraw, fulfillmentRecords)
		env.assertSchedulingState(t, noPrivacyWithdraw, tc.expected)
	}
}

func TestContextualScheduler_CloseDormantTimelockAccount(t *testing.T) {
	simulateOpeningSourceAccount := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == closeDormantAccount.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOpeningDestinationAccount := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == *closeDormantAccount.Destination {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOneTreasuryPayment := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == closeDormantAccount.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateAllTreasuryPayments := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == closeDormantAccount.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}
	}

	forceSimulateAllTreasuryPayments := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == closeDormantAccount.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
			}
		}
	}

	forceSimulateOneTemporaryPrivateTransfer := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && fulfillmentRecord.Source == closeDormantAccount.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
				break
			}
		}
	}

	forceSimulateAllTemporaryPrivateTransfer := func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && fulfillmentRecord.Source == closeDormantAccount.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
			},
			expected: false,
		},
		{
			// Treasury payments not confirmed
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Subset of treasury payments confirmed
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
				simulateOneTreasuryPayment(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Temporary private transfers not confirmed
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateAllTreasuryPayments(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Subset of temporary private transfers not confirmed
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateAllTreasuryPayments(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateOneTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Destination account not opened
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateAllTreasuryPayments(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Source user account not opened
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTreasuryPayments(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, closeDormantAccount *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningSourceAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateOpeningDestinationAccount(env, closeDormantAccount, fulfillmentRecords)
				simulateAllTreasuryPayments(env, closeDormantAccount, fulfillmentRecords)
				forceSimulateAllTemporaryPrivateTransfer(env, closeDormantAccount, fulfillmentRecords)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(1)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(1)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-2", Quantity: kin.ToQuarks(2)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-2", Quantity: kin.ToQuarks(2)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-1", Quantity: kin.ToQuarks(3)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: alice, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "alice-incoming-1", Quantity: kin.ToQuarks(3)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-2", Quantity: kin.ToQuarks(4)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: alice, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "alice-incoming-2", Quantity: kin.ToQuarks(4)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		// Find and pick Bob's 1 Kin bucket, which includes all combinations of
		// fulfillments a close dormant account action might be blocked on.
		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.CloseDormantTimelockAccount)
		require.True(t, len(filtered) > 7)
		closeDormantAccount := filtered[7]
		assert.Equal(t, "bob-bucket-1-kin", closeDormantAccount.Source)

		env.simulateMarkingAccountAsDormantFromAction(t, closeDormantAccount.Intent, closeDormantAccount.ActionId)
		env.assertSchedulingState(t, closeDormantAccount, false)

		tc.simulation(env, closeDormantAccount, fulfillmentRecords)
		env.assertSchedulingState(t, closeDormantAccount, tc.expected)
	}
}

func TestContextualScheduler_PrivateTransfer_TemporaryPrivacyFlow(t *testing.T) {
	simulateOpeningUserAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOpeningCommitmentVault := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.Source == *transfer.Destination {
				switch fulfillmentRecord.FulfillmentType {
				case fulfillment.InitializeCommitmentProof, fulfillment.UploadCommitmentProof, fulfillment.VerifyCommitmentProof, fulfillment.OpenCommitmentVault:
					env.assertSchedulingState(t, fulfillmentRecord, true)
					env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				}
			}
		}
	}

	simulatePrivacyUpgradeDeadlineMet := func(env *schedulerTestEnv, transfer *fulfillment.Record) {
		timelockRecord, err := env.data.GetTimelockByVault(env.ctx, transfer.Source)
		require.NoError(t, err)

		timelockRecord.VaultState = timelock_token_v1.StateUnlocked
		timelockRecord.Block += 1
		require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))
	}

	simulatePrivacyUpgraded := func(env *schedulerTestEnv, transfer *fulfillment.Record) {
		env.simulatePrivacyUpgrades(t, []*fulfillment.Record{transfer})
	}

	simulateOneTreasuryPayment := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateAllTreasuryPayments := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {},
			expected:   false,
		},
		{
			// Privacy upgrade deadline not met
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Commitment vault isn't opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				simulatePrivacyUpgradeDeadlineMet(env, transfer)
			},
			expected: false,
		},
		{
			// Subset of treasury payments have been confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateOneTreasuryPayment(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
				simulatePrivacyUpgradeDeadlineMet(env, transfer)
			},
			expected: false,
		},
		{
			// No treasury payments have been confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
				simulatePrivacyUpgradeDeadlineMet(env, transfer)
			},
			expected: false,
		},
		{
			// Privacy was upgraded (despite deadline and other factors being met)
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
				simulatePrivacyUpgradeDeadlineMet(env, transfer)
				simulatePrivacyUpgraded(env, transfer)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
				simulatePrivacyUpgradeDeadlineMet(env, transfer)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-2", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-2", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-2", Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		// Pick something near the end where we have a transfer to the temporary
		// private transfer source account.
		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.TemporaryPrivacyTransferWithAuthority)
		var temporaryPrivateTransfer *fulfillment.Record
		for i := len(filtered) - 1; i >= 0; i-- {
			if filtered[i].Source == "bob-bucket-1-kin" {
				temporaryPrivateTransfer = filtered[i]
				break
			}
		}
		require.NotNil(t, temporaryPrivateTransfer)

		commitmentRecord, err := env.data.GetCommitmentByVault(env.ctx, *temporaryPrivateTransfer.Destination)
		require.NoError(t, err)
		fulfillmentRecords = env.simulateOpeningCommitmentVault(t, commitmentRecord, fulfillmentRecords)

		env.assertSchedulingState(t, temporaryPrivateTransfer, false)

		tc.simulation(env, temporaryPrivateTransfer, fulfillmentRecords)
		env.assertSchedulingState(t, temporaryPrivateTransfer, tc.expected)
	}
}

func TestContextualScheduler_PrivateTransfer_PermanentPrivacyFlow(t *testing.T) {
	simulateOpeningUserAccount := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateOpeningCommitmentVault := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.Source == *transfer.Destination {
				switch fulfillmentRecord.FulfillmentType {
				case fulfillment.InitializeCommitmentProof, fulfillment.UploadCommitmentProof, fulfillment.VerifyCommitmentProof, fulfillment.OpenCommitmentVault:
					env.assertSchedulingState(t, fulfillmentRecord, true)
					env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				}
			}
		}
	}

	simulateOneTreasuryPayment := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	simulateAllTreasuryPayments := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}
	}

	forceSimulateAllTreasuryPayments := func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && *fulfillmentRecord.Destination == transfer.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				env.data.UpdateFulfillment(env.ctx, fulfillmentRecord)
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {},
			expected:   false,
		},
		{
			// User account hasn't been created
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
				forceSimulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Commitment vault hasn't been opened
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// No treasury payments have been confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Subset of treasury payments have been confirmed
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateOneTreasuryPayment(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, transfer *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, transfer, fulfillmentRecords)
				simulateAllTreasuryPayments(env, transfer, fulfillmentRecords)
				simulateOpeningCommitmentVault(env, transfer, fulfillmentRecords)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-2", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-2", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-2", Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})
		fulfillmentRecords = env.simulateSavingTreasuryRecentRoot(t, fulfillmentRecords)
		fulfillmentRecords, blockCommitmentRecord := env.simulatePrivacyUpgrades(t, fulfillmentRecords)
		fulfillmentRecords = env.simulateOpeningCommitmentVault(t, blockCommitmentRecord, fulfillmentRecords)

		// Pick something near the end where we have a transfer to the permanent
		// private transfer source account.
		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.PermanentPrivacyTransferWithAuthority)
		var permanentPrivateTransfer *fulfillment.Record
		for i := len(filtered) - 1; i >= 0; i-- {
			if filtered[i].Source == "bob-bucket-1-kin" {
				permanentPrivateTransfer = filtered[i]
				break
			}
		}
		require.NotNil(t, permanentPrivateTransfer)

		env.assertSchedulingState(t, permanentPrivateTransfer, false)

		tc.simulation(env, permanentPrivateTransfer, fulfillmentRecords)
		env.assertSchedulingState(t, permanentPrivateTransfer, tc.expected)
	}
}

func TestContextualScheduler_ClosingEmptyAccounts(t *testing.T) {
	simulateOpeningUserAccount := func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.Source == closure.Source {
				env.assertSchedulingState(t, fulfillmentRecord, true)
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
	}

	forceSimulateInboundTransfer := func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw && *fulfillmentRecord.Destination == closure.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				require.NoError(t, env.data.UpdateFulfillment(env.ctx, fulfillmentRecord))
				break
			}
		}
	}

	forceSimulateOneOutboundTransfers := func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && fulfillmentRecord.Source == closure.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				require.NoError(t, env.data.UpdateFulfillment(env.ctx, fulfillmentRecord))
				break
			}
		}
	}

	forceSimulateAllOutboundTransfers := func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && fulfillmentRecord.Source == closure.Source {
				fulfillmentRecord.State = fulfillment.StateConfirmed
				require.NoError(t, env.data.UpdateFulfillment(env.ctx, fulfillmentRecord))
			}
		}
	}

	for _, tc := range []struct {
		simulation func(*schedulerTestEnv, *fulfillment.Record, []*fulfillment.Record)
		expected   bool
	}{
		{
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {},
			expected:   false,
		},
		{
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, closure, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Outbound transfers haven't been confirmend
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, closure, fulfillmentRecords)
				forceSimulateInboundTransfer(env, closure, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Subset of outbound transfers haven't been confirmed
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, closure, fulfillmentRecords)
				forceSimulateInboundTransfer(env, closure, fulfillmentRecords)
				forceSimulateOneOutboundTransfers(env, closure, fulfillmentRecords)
			},
			expected: false,
		},
		{
			// Inbound transfer hasn't been confirmed
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, closure, fulfillmentRecords)
				forceSimulateAllOutboundTransfers(env, closure, fulfillmentRecords)
			},
			expected: false,
		},
		{
			simulation: func(env *schedulerTestEnv, closure *fulfillment.Record, fulfillmentRecords []*fulfillment.Record) {
				simulateOpeningUserAccount(env, closure, fulfillmentRecords)
				forceSimulateInboundTransfer(env, closure, fulfillmentRecords)
				forceSimulateAllOutboundTransfers(env, closure, fulfillmentRecords)
			},
			expected: true,
		},
	} {
		alice := "alice"
		bob := "bob"
		env := setupSchedulerEnv(t, &testOverrides{})
		intentRecords := []*intent.Record{
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
			{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
			{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		}
		fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

		filtered := filterFulfillmentsByType(fulfillmentRecords, fulfillment.CloseEmptyTimelockAccount)
		require.Len(t, filtered, 1)
		closeEmptyAccount := filtered[0]
		env.assertSchedulingState(t, closeEmptyAccount, false)

		tc.simulation(env, closeEmptyAccount, fulfillmentRecords)
		env.assertSchedulingState(t, closeEmptyAccount, tc.expected)
	}
}

func TestContextualScheduler_SaveRecentRoot(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	fulfillmentRecords := env.simulateSavingTreasuryRecentRoot(t, nil)
	require.Len(t, fulfillmentRecords, 1)
	env.assertSchedulingState(t, fulfillmentRecords[0], true)

	transferWithCommitmentBeforeSave := getBaseTestFulfillmentRecord(t)
	transferWithCommitmentBeforeSave.IntentOrderingIndex = fulfillmentRecords[0].IntentOrderingIndex - 1

	transferWithCommitmentAfterSave := getBaseTestFulfillmentRecord(t)
	transferWithCommitmentAfterSave.IntentOrderingIndex = fulfillmentRecords[0].IntentOrderingIndex + 1

	for _, fulfillmentRecord := range []*fulfillment.Record{
		transferWithCommitmentBeforeSave,
		transferWithCommitmentAfterSave,
	} {
		fulfillmentRecord.IntentType = intent.SendPrivatePayment
		fulfillmentRecord.ActionType = action.PrivateTransfer
		fulfillmentRecord.FulfillmentType = fulfillment.TransferWithCommitment
		fulfillmentRecord.Source = fulfillmentRecords[0].Source
		fulfillmentRecord.Destination = pointer.String(testutil.NewRandomAccount(t).PublicKey().ToBase58())
	}
	require.NoError(t, env.data.PutAllFulfillments(env.ctx, transferWithCommitmentBeforeSave, transferWithCommitmentAfterSave))

	env.assertSchedulingState(t, fulfillmentRecords[0], false)

	transferWithCommitmentBeforeSave.State = fulfillment.StateConfirmed
	require.NoError(t, env.data.UpdateFulfillment(env.ctx, transferWithCommitmentBeforeSave))
	env.assertSchedulingState(t, fulfillmentRecords[0], true)
}

func TestContextualScheduler_CommitmentVaultManagement_HappyPath(t *testing.T) {
	env := setupSchedulerEnv(t, &testOverrides{})
	_, commitmentRecord := env.simulatePrivacyUpgrades(t, nil)
	fulfillmentRecords := env.simulateOpeningCommitmentVault(t, commitmentRecord, nil)

	expectedFulfillmentTypeToOpenVault := []fulfillment.Type{
		fulfillment.InitializeCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.VerifyCommitmentProof,
		fulfillment.OpenCommitmentVault,
	}
	require.Len(t, fulfillmentRecords, len(expectedFulfillmentTypeToOpenVault))

	for i, scheduledFulfillment := range fulfillmentRecords {
		assert.Equal(t, expectedFulfillmentTypeToOpenVault[i], scheduledFulfillment.FulfillmentType)
		env.assertSchedulingState(t, scheduledFulfillment, true)

		for j, otherFulfillmentRecord := range fulfillmentRecords {
			if i <= j {
				continue
			}

			env.assertSchedulingState(t, otherFulfillmentRecord, false)
		}

		scheduledFulfillment.State = fulfillment.StateConfirmed
		require.NoError(t, env.data.UpdateFulfillment(env.ctx, scheduledFulfillment))
	}

	fulfillmentRecords = env.simulateClosingCommitmentVault(t, commitmentRecord, nil)
	require.Len(t, fulfillmentRecords, 1)
	env.assertSchedulingState(t, fulfillmentRecords[0], true)
}

func TestContextualScheduler_CommitmentVaultManagement_SafetyMeasures(t *testing.T) {
	// Not scheduable if the commitment vault open process wasn't complete
	expectedFulfillmentTypeToOpenVault := []fulfillment.Type{
		fulfillment.InitializeCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.UploadCommitmentProof,
		fulfillment.VerifyCommitmentProof,
		fulfillment.OpenCommitmentVault,
	}
	for i := range expectedFulfillmentTypeToOpenVault {
		env := setupSchedulerEnv(t, &testOverrides{})
		_, commitmentRecord := env.simulatePrivacyUpgrades(t, nil)
		openFulfillmentRecords := env.simulateOpeningCommitmentVault(t, commitmentRecord, nil)
		require.Len(t, openFulfillmentRecords, len(expectedFulfillmentTypeToOpenVault))

		for j := 0; j < i; j++ {
			env.simulateConfirmedFulfillment(t, openFulfillmentRecords[j])
		}

		closeFulfillmentRecords := env.simulateClosingCommitmentVault(t, commitmentRecord, nil)
		env.assertSchedulingState(t, closeFulfillmentRecords[0], false)
	}

	// Not scheduable if we missed or are waiting on cashing in a cheque
	for _, tc := range []struct {
		fulfillmentType fulfillment.Type
		state           fulfillment.State
	}{
		{fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StateUnknown},
		{fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.StatePending},
		{fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.StateUnknown},
		{fulfillment.PermanentPrivacyTransferWithAuthority, fulfillment.StatePending},
	} {
		env := setupSchedulerEnv(t, &testOverrides{})
		_, commitmentRecord := env.simulatePrivacyUpgrades(t, nil)

		complication := getBaseTestFulfillmentRecord(t)
		complication.IntentType = intent.SendPrivatePayment
		complication.ActionType = action.PrivateTransfer
		complication.FulfillmentType = tc.fulfillmentType
		complication.State = tc.state
		complication.Destination = &commitmentRecord.Vault
		require.NoError(t, env.data.PutAllFulfillments(env.ctx, complication))

		closeFulfillmentRecords := env.simulateClosingCommitmentVault(t, commitmentRecord, nil)
		env.assertSchedulingState(t, closeFulfillmentRecords[0], false)
	}
}

func TestContextualScheduler_PrivacyV3Demonstration_Simple(t *testing.T) {
	defer testutil.DisableLogging()

	alice := "alice"
	bob := "bob"
	charlie := "charlie"

	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{
		// Alice, Bob and Charlie open their accounts
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: charlie, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},

		// Alice privately sends Bob 42 Kin to his temporary incoming account
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},

		// Bob privately receives 42 Kin from his temporary incoming account
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},

		// Bob privately sends 42 Kin to his primary account, and then withdraws it Charlie
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-primary", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.SendPublicPayment, InitiatorOwnerAccount: bob, SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{DestinationTokenAccount: "charlie-primary", Quantity: kin.ToQuarks(42)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{
		includeReorganizations: false,
	})

	// Open Alice's bucket accounts because we've setup our scenario in an invalid
	// way. They don't receive funds, but are sending them out.
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && strings.Contains(fulfillmentRecord.Source, "alice-bucket-1") {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}

	// First set of scheduled fulfillments are to open accounts that will receive funds
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		var expectedState bool
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.InitializeLockedTimelockAccount:
			switch fulfillmentRecord.Source {
			case "alice-primary", "bob-primary", "charlie-primary",
				"alice-outgoing-1", "bob-outgoing-1", "bob-incoming-1",
				"bob-bucket-1-kin", "bob-bucket-10-kin":
				expectedState = true
			}
		}
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
	}

	printForTest("")
	printForTest("simulating all account opens on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// All treasury payments should be scheduled now that destination accounts exist
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment)
	}

	printForTest("")
	printForTest("simulating all payments from treasury on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Alice's and Bob's temporary outgoing accounts are fully funded, so the private
	// payments can be completed with withdraws to destinations.
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw)
	}

	printForTest("")
	printForTest("simulating all no privacy withdraws on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Bob's primary account is funded, so he can now send his public payment to
	// Charlie.
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyTransferWithAuthority)
	}

	printForTest("")
	printForTest("simulating all no privacy transfers on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyTransferWithAuthority {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Nothing is scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating creating a block on the treasury pool with server process")
	fulfillmentRecords = env.simulateSavingTreasuryRecentRoot(t, fulfillmentRecords)
	printForTest("")

	// Fulfillment to save recent root is now scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.SaveRecentRoot)
	}

	printForTest("")
	printForTest("simulating saving recent root on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.SaveRecentRoot {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Nothing is scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating privacy upgrades with server and client processes")
	fulfillmentRecords, blockCommitmentRecord := env.simulatePrivacyUpgrades(t, fulfillmentRecords)
	printForTest("")

	// Nothing is scheduled yet because the commitment vault hasn't been opened
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating opening block commitment vault with server process")
	fulfillmentRecords = env.simulateOpeningCommitmentVault(t, blockCommitmentRecord, fulfillmentRecords)
	printForTest("")

	// Initializing the commitment proof is now scheduled since the commitment
	// worker said it was ok to do so
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.InitializeCommitmentProof)
	}

	printForTest("")
	printForTest("simulating initializing commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeCommitmentProof {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	for i := 0; i < 3; i++ {
		// The commitment proof is initialized, so we can schedule uploading the proof
		// serially.
		env.printScheduledFulfillments(t, fulfillmentRecords)
		for _, fulfillmentRecord := range fulfillmentRecords {
			var expectedState bool
			switch fulfillmentRecord.FulfillmentType {
			case fulfillment.UploadCommitmentProof:
				expectedState = (int(fulfillmentRecord.FulfillmentOrderingIndex) == i+1)
			}
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
		}

		printForTest("")
		printForTest("simulating uploading commitment vault proof on the blockchain")
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.UploadCommitmentProof && fulfillmentRecord.State != fulfillment.StateConfirmed {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		printForTest("")
	}

	// The commitment proof is uploaded, so we can schedule the fulfillment to
	// verify it
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.VerifyCommitmentProof)
	}

	printForTest("")
	printForTest("simulating verifying commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.VerifyCommitmentProof {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// The commitment proof is verified, so we can schedule the fulfillment to
	// open the commitment vault
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.OpenCommitmentVault)
	}

	printForTest("")
	printForTest("simulating opening commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.OpenCommitmentVault {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// The commitment vault is opened, so we can schedule the upgraded private
	// transfers that target it as a destination.
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.PermanentPrivacyTransferWithAuthority)
	}

	printForTest("")
	printForTest("simulating cashing in all cheques with permanent privacy on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority {
			env.simulateRevokedFulfillment(t, fulfillmentRecord)
		}

		if fulfillmentRecord.FulfillmentType == fulfillment.PermanentPrivacyTransferWithAuthority {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Bob's temporary incoming account has no funds, so we can schedule the fulfillment
	// to close it
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.CloseEmptyTimelockAccount)
	}

	printForTest("")
	printForTest("simulating closing Bob's temporary incoming account on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.CloseEmptyTimelockAccount && fulfillmentRecord.Source == "bob-incoming-1" {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Nothing is scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating closing block commitment vault with server process")
	fulfillmentRecords = env.simulateClosingCommitmentVault(t, blockCommitmentRecord, fulfillmentRecords)
	printForTest("")

	// The fulfillment to close the commitment vault can now be scheduled since the
	// commitment worker said it was ok to so
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.CloseCommitmentVault)
	}

	printForTest("")
	printForTest("simulating closing block commitment vault on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.CloseCommitmentVault {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Nothing else is scheduled. We're done
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}
}

func TestContextualScheduler_PrivacyV3Demonstration_SlightlyMoreComplexAndWayMoreVerbose(t *testing.T) {
	defer testutil.DisableLogging()

	alice := "alice"
	bob := "bob"
	charlie := "charlie"

	env := setupSchedulerEnv(t, &testOverrides{})
	intentRecords := []*intent.Record{
		// Alice, Bob and Charlie open their accounts
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: alice, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: bob, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},
		{IntentType: intent.OpenAccounts, InitiatorOwnerAccount: charlie, OpenAccountsMetadata: &intent.OpenAccountsMetadata{}},

		// Alice privately transacts with Bob and Charlie sending funds to their temporary incoming accounts
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: alice, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "charlie-incoming-1", Quantity: kin.ToQuarks(123)}},

		// Bob and Charlie privately receive Kin from their temporary incoming accounts
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-1", Quantity: kin.ToQuarks(84)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: charlie, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "charlie-incoming-1", Quantity: kin.ToQuarks(123)}},

		// Bob privately sends Alice back some of her funds to her temporary incoming account
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: bob, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "alice-incoming-1", Quantity: kin.ToQuarks(42)}},

		// Charlie privately sends Bob funds to his tmeporary incoming account
		{IntentType: intent.SendPrivatePayment, InitiatorOwnerAccount: charlie, SendPrivatePaymentMetadata: &intent.SendPrivatePaymentMetadata{DestinationTokenAccount: "bob-incoming-2", Quantity: kin.ToQuarks(999)}},

		// Alice and Bob privately receive Kin from their temporary incoming accounts
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: alice, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "alice-incoming-1", Quantity: kin.ToQuarks(42)}},
		{IntentType: intent.ReceivePaymentsPrivately, InitiatorOwnerAccount: bob, ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{Source: "bob-incoming-2", Quantity: kin.ToQuarks(999)}},
	}
	fulfillmentRecords := env.setupSchedulerTest(t, intentRecords, schedulerTestOptions{})

	// Open Alice's bucket accounts because we've setup our scenario in an invalid
	// way. They don't receive funds, but are sending them out.
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && strings.Contains(fulfillmentRecord.Source, "alice-bucket-1") {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}

	openedAccounts := map[string]struct{}{
		"alice-bucket-1-kin":   {},
		"alice-bucket-10-kin":  {},
		"alice-bucket-100-kin": {},
	}
	for {
		// Fulfillments to open accounts that will receive funds will be scheduled.
		// As they are opened, fulfillments for treasury payments that target the
		// opened account also become scheduable.
		env.printScheduledFulfillments(t, fulfillmentRecords)
		for _, fulfillmentRecord := range fulfillmentRecords {
			var expectedState bool
			switch fulfillmentRecord.FulfillmentType {
			case fulfillment.InitializeLockedTimelockAccount:
				switch fulfillmentRecord.Source {
				case
					// Primary accounts
					"alice-primary", "bob-primary", "charlie-primary",
					// Alice's transfer accounts
					"alice-incoming-1", "alice-outgoing-1", "alice-outgoing-2", "alice-outgoing-3",
					// Bob's transfer accounts
					"bob-incoming-1", "bob-incoming-2", "bob-outgoing-1",
					// Charlie's transfer accounts
					"charlie-incoming-1", "charlie-outgoing-1",
					// Bob's bucket accounts
					"bob-bucket-1-kin", "bob-bucket-10-kin", "bob-bucket-100-kin",
					// Charlie's bucket accounts
					"charlie-bucket-1-kin", "charlie-bucket-10-kin", "charlie-bucket-100-kin":
					expectedState = true
				}
			case fulfillment.TransferWithCommitment:
				_, expectedState = openedAccounts[*fulfillmentRecord.Destination]
			}
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
		}

		var accountOpened string
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.InitializeLockedTimelockAccount && fulfillmentRecord.State != fulfillment.StateConfirmed {
				accountOpened = fulfillmentRecord.Source
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		if len(accountOpened) == 0 {
			break
		}

		printForTest("")
		printForTest("simulating opening %s account on the blockchain\n", accountOpened)
		printForTest("")

		openedAccounts[accountOpened] = struct{}{}
	}

	usedTemporaryOutgoingAccounts := map[string]struct{}{
		"alice-outgoing-1":   {},
		"alice-outgoing-2":   {},
		"alice-outgoing-3":   {},
		"bob-outgoing-1":     {},
		"charlie-outgoing-1": {},
	}
	temporaryOutgoingAccountsRequiringFunding := usedTemporaryOutgoingAccounts
	for {
		// As fulfillments for treasury payments are confirmed and temporary
		// outgoing accounts become fully funded, the fulfillments to pay
		// the temporary incoming accounts become scheduable.
		for _, fulfillmentRecord := range fulfillmentRecords {
			var expectedState bool
			switch fulfillmentRecord.FulfillmentType {
			case fulfillment.TransferWithCommitment:
				expectedState = true
			case fulfillment.NoPrivacyWithdraw:
				_, requiresFunding := temporaryOutgoingAccountsRequiringFunding[fulfillmentRecord.Source]
				expectedState = !requiresFunding
			}
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
		}

		var destination string
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TransferWithCommitment && fulfillmentRecord.State != fulfillment.StateConfirmed {
				destination = *fulfillmentRecord.Destination
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		if len(destination) == 0 {
			break
		}

		printForTest("")
		printForTest("simulating next payment from treasury to %s on the blockchain\n", destination)
		printForTest("")

		temporaryOutgoingAccountsRequiringFunding = make(map[string]struct{})
		for temporaryOutgoingAccount := range usedTemporaryOutgoingAccounts {
			for _, fulfillmentRecord := range fulfillmentRecords {
				if fulfillmentRecord.State.IsTerminal() {
					continue
				}

				if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
					continue
				}

				if *fulfillmentRecord.Destination == temporaryOutgoingAccount {
					temporaryOutgoingAccountsRequiringFunding[temporaryOutgoingAccount] = struct{}{}
				}
			}
		}

		env.printScheduledFulfillments(t, fulfillmentRecords)
	}

	for temporaryOutgoingAccount := range usedTemporaryOutgoingAccounts {
		// Fulfillments to transfer between temporary outgoing to temporary incoming
		// acounts are scheduled. As they're confirmed, nothing else becomes unblocked.
		for _, fulfillmentRecord := range fulfillmentRecords {
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw)
		}

		printForTest("")
		printForTest("simulating transfer of funds from temporary outgoing account %s on the blockchain\n", temporaryOutgoingAccount)
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.NoPrivacyWithdraw && fulfillmentRecord.Source == temporaryOutgoingAccount {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}
		printForTest("")

		env.printScheduledFulfillments(t, fulfillmentRecords)
	}

	printForTest("")
	printForTest("simulating creating a block on the treasury pool with server process")
	fulfillmentRecords = env.simulateSavingTreasuryRecentRoot(t, fulfillmentRecords)
	printForTest("")

	// Fulfillment to save recent root is now scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.SaveRecentRoot)
	}

	printForTest("")
	printForTest("simulating saving recent root on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.SaveRecentRoot {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// Nothing is scheduled
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating privacy upgrades with server and client processes")
	fulfillmentRecords, blockCommitmentRecord := env.simulatePrivacyUpgrades(t, fulfillmentRecords)
	printForTest("")

	// Nothing is scheduled yet because the commitment vault hasn't been opened
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, false)
	}

	printForTest("")
	printForTest("simulating opening block commitment vault with server process")
	fulfillmentRecords = env.simulateOpeningCommitmentVault(t, blockCommitmentRecord, fulfillmentRecords)
	printForTest("")

	// Initializing the commitment proof is now scheduled since the commitment
	// worker said it was ok to do so
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.InitializeCommitmentProof)
	}

	printForTest("")
	printForTest("simulating initializing commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.InitializeCommitmentProof {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	for i := 0; i < 3; i++ {
		/// The commitment proof is initialized, so we can schedule uploading the proof
		// serially.
		env.printScheduledFulfillments(t, fulfillmentRecords)
		for _, fulfillmentRecord := range fulfillmentRecords {
			var expectedState bool
			switch fulfillmentRecord.FulfillmentType {
			case fulfillment.UploadCommitmentProof:
				expectedState = (int(fulfillmentRecord.FulfillmentOrderingIndex) == i+1)
			}
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
		}

		printForTest("")
		printForTest("simulating uploading commitment vault proof on the blockchain")
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.UploadCommitmentProof && fulfillmentRecord.State != fulfillment.StateConfirmed {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		printForTest("")
	}

	// The commitment proof is uploaded, so we can schedule the fulfillment to
	// verify it
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.VerifyCommitmentProof)
	}

	printForTest("")
	printForTest("simulating verifying commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.VerifyCommitmentProof {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	// The commitment proof is verified, so we can schedule the fulfillment to
	// open the commitment vault
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.OpenCommitmentVault)
	}

	printForTest("")
	printForTest("simulating opening commitment vault proof on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.OpenCommitmentVault {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	var usedTemporarIncomingAccounts = map[string]struct{}{
		"bob-incoming-1":     {},
		"bob-incoming-2":     {},
		"alice-incoming-1":   {},
		"charlie-incoming-1": {},
	}
	temporaryOutgoingAccountsWithFunds := usedTemporarIncomingAccounts
	for {
		// The commitment vault is opened, so we can schedule the upgraded private
		// transfers that target it as a destination. As temporary incoming accounts
		// are drained of funds, the fulfillments to close the account become scheduable.
		env.printScheduledFulfillments(t, fulfillmentRecords)
		for _, fulfillmentRecord := range fulfillmentRecords {
			var expectedState bool
			switch fulfillmentRecord.FulfillmentType {
			case fulfillment.PermanentPrivacyTransferWithAuthority:
				expectedState = true
			case fulfillment.CloseEmptyTimelockAccount:
				_, hasFunds := temporaryOutgoingAccountsWithFunds[fulfillmentRecord.Source]
				expectedState = !hasFunds
			}
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
		}

		var source string
		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && fulfillmentRecord.State != fulfillment.StateRevoked {
				env.simulateRevokedFulfillment(t, fulfillmentRecord)
			}

			if fulfillmentRecord.FulfillmentType == fulfillment.PermanentPrivacyTransferWithAuthority && fulfillmentRecord.State != fulfillment.StateConfirmed {
				source = fulfillmentRecord.Source
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
				break
			}
		}
		if len(source) == 0 {
			break
		}

		printForTest("")
		printForTest("simulating cashing in next cheque with permanent privacy from %s on the blockchain", source)
		printForTest("")

		temporaryOutgoingAccountsWithFunds = make(map[string]struct{})
		for temporaryIncomingAccount := range usedTemporarIncomingAccounts {
			for _, fulfillmentRecord := range fulfillmentRecords {
				if fulfillmentRecord.State.IsTerminal() {
					continue
				}

				if fulfillmentRecord.FulfillmentType != fulfillment.PermanentPrivacyTransferWithAuthority {
					continue
				}

				if fulfillmentRecord.Source == temporaryIncomingAccount {
					temporaryOutgoingAccountsWithFunds[temporaryIncomingAccount] = struct{}{}
				}
			}
		}
	}

	printForTest("")
	printForTest("simulating closing block commitment vault with server process")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority && *fulfillmentRecord.Destination == blockCommitmentRecord.Vault {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	fulfillmentRecords = env.simulateClosingCommitmentVault(t, blockCommitmentRecord, fulfillmentRecords)
	printForTest("")

	// The fulfillment to close the commitment vault can now be scheduled since the
	// commitment worker said it was ok to so
	env.printScheduledFulfillments(t, fulfillmentRecords)
	for _, fulfillmentRecord := range fulfillmentRecords {
		var expectedState bool
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.CloseCommitmentVault, fulfillment.CloseEmptyTimelockAccount:
			expectedState = true
		}
		env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, expectedState)
	}

	printForTest("")
	printForTest("simulating closing block commitment vault on the blockchain")
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.CloseCommitmentVault {
			env.simulateConfirmedFulfillment(t, fulfillmentRecord)
		}
	}
	printForTest("")

	for temporaryIncomingAccount := range usedTemporarIncomingAccounts {
		env.printScheduledFulfillments(t, fulfillmentRecords)
		for _, fulfillmentRecord := range fulfillmentRecords {
			env.assertSchedulingStateIfNotConfirmed(t, fulfillmentRecord, fulfillmentRecord.FulfillmentType == fulfillment.CloseEmptyTimelockAccount)
		}

		printForTest("")
		printForTest("simulating closing temporary incoming account %s on the blockchain", temporaryIncomingAccount)

		for _, fulfillmentRecord := range fulfillmentRecords {
			if fulfillmentRecord.FulfillmentType == fulfillment.CloseEmptyTimelockAccount && fulfillmentRecord.Source == temporaryIncomingAccount {
				env.simulateConfirmedFulfillment(t, fulfillmentRecord)
			}
		}

		printForTest("")
	}
}

type schedulerTestEnv struct {
	ctx            context.Context
	data           code_data.Provider
	scheduler      *contextualScheduler
	handlersByType map[fulfillment.Type]FulfillmentHandler
	subsidizer     *common.Account
	nextSlot       uint64
}

func setupSchedulerEnv(t *testing.T, overrides *testOverrides) *schedulerTestEnv {
	if overrides.maxGlobalFailedFulfillments == 0 {
		overrides.maxGlobalFailedFulfillments = defaultMaxGlobalFailedFulfillments
	}

	configProvider := withManualTestOverrides(overrides)

	data := code_data.NewTestDataProvider()
	env := &schedulerTestEnv{
		ctx:            context.Background(),
		data:           data,
		scheduler:      NewContextualScheduler(data, configProvider).(*contextualScheduler),
		handlersByType: getFulfillmentHandlers(data, configProvider),
		subsidizer:     testutil.SetupRandomSubsidizer(t, data),
	}

	usdRate := &currency.MultiRateRecord{
		Time: time.Now(),
		Rates: map[string]float64{
			"usd": 1.0,
		},
	}
	require.NoError(t, env.data.ImportExchangeRates(env.ctx, usdRate))

	env.scheduler.includeSubsidizerChecks = false

	return env
}

type schedulerTestOptions struct {
	includeReorganizations bool
	treasuryPoolFunds      uint64
}

func (e *schedulerTestEnv) setupSchedulerTest(t *testing.T, intentRecords []*intent.Record, opts schedulerTestOptions) []*fulfillment.Record {
	codeFeeCollector := testutil.NewRandomAccount(t).PublicKey().ToBase58()
	feeAmount := kin.ToQuarks(1)

	treasuryPoolRecord := &treasury.Record{
		DataVersion:      splitter_token.DataVersion1,
		Name:             "test-pool",
		Address:          "test-pool-address",
		Vault:            "test-pool-vault",
		Authority:        "code",
		MerkleTreeLevels: 63,
		CurrentIndex:     0,
		HistoryListSize:  1,
		HistoryList:      []string{"unused"},
		SolanaBlock:      123,
		State:            treasury.TreasuryPoolStateAvailable,
	}
	require.NoError(t, e.data.SaveTreasuryPool(e.ctx, treasuryPoolRecord))

	fundingLevel := kin.ToQuarks(1000000)
	if opts.treasuryPoolFunds > 0 {
		fundingLevel = opts.treasuryPoolFunds
	}
	treasuryPoolFundingRecord := &treasury.FundingHistoryRecord{
		Vault:         treasuryPoolRecord.Vault,
		DeltaQuarks:   int64(fundingLevel),
		TransactionId: generateRandomSignature(),
		State:         treasury.FundingStateConfirmed,
		CreatedAt:     time.Now(),
	}
	require.NoError(t, e.data.SaveTreasuryPoolFunding(e.ctx, treasuryPoolFundingRecord))

	// todo: Make this more elegant
	currentIncomingByUser := make(map[string]int)
	currentOutgoingByUser := make(map[string]int)

	// Fill in any required intents fields, some of which that aren't explicitly
	// needed for tests.
	for _, intentRecord := range intentRecords {
		intentRecord.IntentId = testutil.NewRandomAccount(t).PublicKey().ToBase58()
		intentRecord.State = intent.StatePending

		switch intentRecord.IntentType {
		case intent.OpenAccounts:
		case intent.SendPrivatePayment:
			intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount = testutil.NewRandomAccount(t).PublicKey().ToBase58()

			intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency = currency_lib.USD
			intentRecord.SendPrivatePaymentMetadata.ExchangeRate = 1.0
			intentRecord.SendPrivatePaymentMetadata.NativeAmount = float64(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity))
			intentRecord.SendPrivatePaymentMetadata.UsdMarketValue = float64(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity))
		case intent.ReceivePaymentsPrivately:
			intentRecord.ReceivePaymentsPrivatelyMetadata.UsdMarketValue = float64(kin.FromQuarks(intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity))
		case intent.SendPublicPayment:
			intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount = testutil.NewRandomAccount(t).PublicKey().ToBase58()

			intentRecord.SendPublicPaymentMetadata.ExchangeCurrency = currency_lib.USD
			intentRecord.SendPublicPaymentMetadata.ExchangeRate = 1.0
			intentRecord.SendPublicPaymentMetadata.NativeAmount = float64(kin.FromQuarks(intentRecord.SendPublicPaymentMetadata.Quantity))
			intentRecord.SendPublicPaymentMetadata.UsdMarketValue = float64(kin.FromQuarks(intentRecord.SendPublicPaymentMetadata.Quantity))
		default:
			assert.Fail(t, "unsupported intent type")
		}
		require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))
	}

	// Create the expected set of action records for each intent
	var actionRecords []*action.Record
	for _, intentRecord := range intentRecords {
		var newActionRecords []*action.Record
		switch intentRecord.IntentType {
		case intent.OpenAccounts:
			currentIncomingByUser[intentRecord.InitiatorOwnerAccount] = 1
			currentOutgoingByUser[intentRecord.InitiatorOwnerAccount] = 1

			for _, accountSuffix := range []string{
				"primary",
				"incoming-1",
				"outgoing-1",
				"bucket-1-kin",
				"bucket-10-kin",
				"bucket-100-kin",
			} {
				openAccount := &action.Record{
					ActionType: action.OpenAccount,

					Source: fmt.Sprintf("%s-%s", intentRecord.InitiatorOwnerAccount, accountSuffix),
				}
				newActionRecords = append(newActionRecords, openAccount)

				if accountSuffix != "primary" {
					destination := fmt.Sprintf("%s-primary", intentRecord.InitiatorOwnerAccount)
					closeAccount := &action.Record{
						ActionType: action.CloseDormantAccount,

						Source:      fmt.Sprintf("%s-%s", intentRecord.InitiatorOwnerAccount, accountSuffix),
						Destination: &destination,
					}
					newActionRecords = append(newActionRecords, closeAccount)
				}
			}
		case intent.SendPrivatePayment:
			assert.True(t, intentRecord.SendPrivatePaymentMetadata.Quantity < kin.ToQuarks(1000))

			var bucketActionRecords []*action.Record

			ones := getDigit(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity), 1)
			if ones > 0 {
				destination := fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount])
				quantity := ones * kin.ToQuarks(1)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-bucket-1-kin", intentRecord.InitiatorOwnerAccount),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			tens := getDigit(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity), 2)
			if tens > 0 {
				destination := fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount])
				quantity := tens * kin.ToQuarks(10)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-bucket-10-kin", intentRecord.InitiatorOwnerAccount),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			hundreds := getDigit(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity), 3)
			if hundreds > 0 {
				destination := fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount])
				quantity := hundreds * kin.ToQuarks(100)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-bucket-100-kin", intentRecord.InitiatorOwnerAccount),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			if opts.includeReorganizations {
				destination := fmt.Sprintf("%s-bucket-100-kin", intentRecord.InitiatorOwnerAccount)
				quantity := kin.ToQuarks(100)
				bucketReorganization := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-bucket-10-kin", intentRecord.InitiatorOwnerAccount),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketReorganization)
			}

			var feePaymentAction *action.Record
			if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
				feePaymentAction = &action.Record{
					ActionType: action.NoPrivacyTransfer,

					Source:      fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount]),
					Destination: &codeFeeCollector,
					Quantity:    &feeAmount,
				}
			}

			outgoingToIncoming := &action.Record{
				ActionType: action.NoPrivacyWithdraw,

				Source:      fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount]),
				Destination: &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount,
				Quantity:    &intentRecord.SendPrivatePaymentMetadata.Quantity,
			}

			openNewOutgoing := &action.Record{
				ActionType: action.OpenAccount,

				Source: fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount]+1),
			}

			primary := fmt.Sprintf("%s-primary", intentRecord.InitiatorOwnerAccount)
			closeNewOutgoing := &action.Record{
				ActionType: action.CloseDormantAccount,

				Source:      fmt.Sprintf("%s-outgoing-%d", intentRecord.InitiatorOwnerAccount, currentOutgoingByUser[intentRecord.InitiatorOwnerAccount]+1),
				Destination: &primary,
			}

			newActionRecords = append(
				newActionRecords,
				bucketActionRecords...,
			)
			if feePaymentAction != nil {
				newActionRecords = append(newActionRecords, feePaymentAction)
			}
			newActionRecords = append(
				newActionRecords,
				outgoingToIncoming,
				openNewOutgoing,
				closeNewOutgoing,
			)

			currentOutgoingByUser[intentRecord.InitiatorOwnerAccount] += 1
		case intent.ReceivePaymentsPrivately:
			assert.True(t, intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity < kin.ToQuarks(1000))

			var bucketActionRecords []*action.Record

			ones := getDigit(kin.FromQuarks(intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity), 1)
			if ones > 0 {
				destination := fmt.Sprintf("%s-bucket-1-kin", intentRecord.InitiatorOwnerAccount)
				quantity := ones * kin.ToQuarks(1)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			tens := getDigit(kin.FromQuarks(intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity), 2)
			if tens > 0 {
				destination := fmt.Sprintf("%s-bucket-10-kin", intentRecord.InitiatorOwnerAccount)
				quantity := tens * kin.ToQuarks(10)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			hundreds := getDigit(kin.FromQuarks(intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity), 3)
			if hundreds > 0 {
				destination := fmt.Sprintf("%s-bucket-100-kin", intentRecord.InitiatorOwnerAccount)
				quantity := uint64(hundreds) * kin.ToQuarks(100)
				bucketToOutgoing := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketToOutgoing)
			}

			if opts.includeReorganizations {
				destination := fmt.Sprintf("%s-bucket-1-kin", intentRecord.InitiatorOwnerAccount)
				quantity := kin.ToQuarks(10)
				bucketReorganization := &action.Record{
					ActionType: action.PrivateTransfer,

					Source:      fmt.Sprintf("%s-bucket-10-kin", intentRecord.InitiatorOwnerAccount),
					Destination: &destination,
					Quantity:    &quantity,
				}
				bucketActionRecords = append(bucketActionRecords, bucketReorganization)
			}

			closeCurrentIncoming := &action.Record{
				ActionType: action.CloseEmptyAccount,

				Source: fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]),
			}

			openNewIncoming := &action.Record{
				ActionType: action.OpenAccount,

				Source: fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]+1),
			}

			primary := fmt.Sprintf("%s-primary", intentRecord.InitiatorOwnerAccount)
			closeNewIncoming := &action.Record{
				ActionType: action.CloseDormantAccount,

				Source:      fmt.Sprintf("%s-incoming-%d", intentRecord.InitiatorOwnerAccount, currentIncomingByUser[intentRecord.InitiatorOwnerAccount]+1),
				Destination: &primary,
			}

			newActionRecords = append(
				newActionRecords,
				bucketActionRecords...,
			)
			newActionRecords = append(
				newActionRecords,
				closeCurrentIncoming,
				openNewIncoming,
				closeNewIncoming,
			)

			currentIncomingByUser[intentRecord.InitiatorOwnerAccount] += 1
		case intent.SendPublicPayment:
			newActionRecords = append(
				newActionRecords,
				&action.Record{
					ActionType: action.NoPrivacyTransfer,

					Source:      fmt.Sprintf("%s-primary", intentRecord.InitiatorOwnerAccount),
					Destination: &intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount,
					Quantity:    &intentRecord.SendPublicPaymentMetadata.Quantity,
				},
			)
		default:
			assert.Fail(t, "unsupported intent type")
		}

		// Fill in remaining action metadata
		for i, newActionRecord := range newActionRecords {
			newActionRecord.Intent = intentRecord.IntentId
			newActionRecord.IntentType = intentRecord.IntentType
			newActionRecord.ActionId = uint32(i)
			if newActionRecord.ActionType != action.CloseDormantAccount {
				newActionRecord.State = action.StatePending
			}
		}
		actionRecords = append(actionRecords, newActionRecords...)
		require.NoError(t, e.data.PutAllActions(e.ctx, newActionRecords...))
	}

	// Create the expected set of fulfillment records for each intent and any
	// necessary state to support them.
	var fulfillmentRecords []*fulfillment.Record
	for _, actionRecord := range actionRecords {
		var intentRecord *intent.Record
		for _, record := range intentRecords {
			if record.IntentId == actionRecord.Intent {
				intentRecord = record
			}
		}
		require.NotNil(t, intentRecord)

		var newFulfillmentRecords []*fulfillment.Record
		switch actionRecord.ActionType {
		case action.OpenAccount:
			var accountType commonpb.AccountType
			if strings.Contains(actionRecord.Source, "primary") {
				accountType = commonpb.AccountType_PRIMARY
			} else if strings.Contains(actionRecord.Source, "incoming") {
				accountType = commonpb.AccountType_TEMPORARY_INCOMING
			} else if strings.Contains(actionRecord.Source, "outgoing") {
				accountType = commonpb.AccountType_TEMPORARY_OUTGOING
			} else if strings.Contains(actionRecord.Source, "bucket-1-kin") {
				accountType = commonpb.AccountType_BUCKET_1_KIN
			} else if strings.Contains(actionRecord.Source, "bucket-10-kin") {
				accountType = commonpb.AccountType_BUCKET_10_KIN
			} else if strings.Contains(actionRecord.Source, "bucket-100-kin") {
				accountType = commonpb.AccountType_BUCKET_100_KIN
			}

			owner := strings.Split(actionRecord.Source, "-")[0]
			authority := fmt.Sprintf("%s-authority", actionRecord.Source)
			if accountType == commonpb.AccountType_PRIMARY {
				authority = owner
			}

			var index uint64
			var err error
			if accountType == commonpb.AccountType_TEMPORARY_INCOMING || accountType == commonpb.AccountType_TEMPORARY_OUTGOING {
				parts := strings.Split(actionRecord.Source, "-")
				index, err = strconv.ParseUint(parts[len(parts)-1], 10, 64)
				require.NoError(t, err)
			}

			accountInfoRecord := &account.Record{
				OwnerAccount:     owner,
				AuthorityAccount: authority,
				TokenAccount:     actionRecord.Source,
				MintAccount:      common.KinMintAccount.PublicKey().ToBase58(),
				AccountType:      accountType,
				Index:            index,
			}
			require.NoError(t, e.data.CreateAccountInfo(e.ctx, accountInfoRecord))

			timelockRecord := &timelock.Record{
				DataVersion:    timelock_token_v1.DataVersion1,
				Address:        fmt.Sprintf("%s-state", actionRecord.Source),
				VaultAddress:   actionRecord.Source,
				VaultOwner:     authority,
				VaultState:     timelock_token_v1.StateUnknown,
				TimeAuthority:  common.GetSubsidizer().PublicKey().ToBase58(),
				CloseAuthority: common.GetSubsidizer().PublicKey().ToBase58(),
				Mint:           common.KinMintAccount.PublicKey().ToBase58(),
				NumDaysLocked:  timelock_token_v1.DefaultNumDaysLocked,
			}
			require.NoError(t, e.data.SaveTimelock(e.ctx, timelockRecord))

			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.InitializeLockedTimelockAccount,

				Source: actionRecord.Source,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.CloseEmptyAccount:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.CloseEmptyTimelockAccount,

				Source: actionRecord.Source,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.CloseDormantAccount:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.CloseDormantTimelockAccount,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				IntentOrderingIndex:      uint64(math.MaxInt64),
				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.NoPrivacyTransfer:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.NoPrivacyWithdraw:
			fulfillmentRecord := &fulfillment.Record{
				FulfillmentType: fulfillment.NoPrivacyWithdraw,

				Source:      actionRecord.Source,
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, fulfillmentRecord)
		case action.PrivateTransfer:
			commitmentRecord := &commitment.Record{
				DataVersion: splitter_token.DataVersion1,

				Address: fmt.Sprintf("commitment-%s-%d", actionRecord.Intent, actionRecord.ActionId),
				Vault:   fmt.Sprintf("commitment-vault-%s-%d", actionRecord.Intent, actionRecord.ActionId),

				Pool:       treasuryPoolRecord.Address,
				RecentRoot: treasuryPoolRecord.GetMostRecentRoot(),
				Transcript: fmt.Sprintf("transcript-%s-%d", actionRecord.Intent, actionRecord.ActionId),

				Destination: *actionRecord.Destination,
				Amount:      *actionRecord.Quantity,

				Intent:   actionRecord.Intent,
				ActionId: actionRecord.ActionId,

				Owner: intentRecord.InitiatorOwnerAccount,
			}
			require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))

			treasuryToBob := &fulfillment.Record{
				FulfillmentType: fulfillment.TransferWithCommitment,

				Source:      treasuryPoolRecord.Vault,
				Destination: actionRecord.Destination,

				FulfillmentOrderingIndex: 0,
			}

			aliceToCommitmentWithTemporaryPrivacy := &fulfillment.Record{
				FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,

				Source:      actionRecord.Source,
				Destination: &commitmentRecord.Vault,

				FulfillmentOrderingIndex: 2000,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, treasuryToBob, aliceToCommitmentWithTemporaryPrivacy)
		default:
			assert.Fail(t, "unsupported action type")
		}

		intentRecord, err := e.data.GetIntent(e.ctx, actionRecord.Intent)
		require.NoError(t, err)

		// Fill in remaining fulfillment metadata
		for _, newFulfillmentRecord := range newFulfillmentRecords {
			newFulfillmentRecord.Intent = actionRecord.Intent
			newFulfillmentRecord.IntentType = actionRecord.IntentType

			newFulfillmentRecord.ActionId = actionRecord.ActionId
			newFulfillmentRecord.ActionType = actionRecord.ActionType

			if newFulfillmentRecord.IntentOrderingIndex == 0 {
				newFulfillmentRecord.IntentOrderingIndex = intentRecord.Id
			}
			newFulfillmentRecord.ActionOrderingIndex = actionRecord.ActionId

			newFulfillmentRecord.Data = []byte("data")
			newFulfillmentRecord.Signature = pointer.String(generateRandomSignature())
			newFulfillmentRecord.Nonce = pointer.String(getRandomNonce())
			newFulfillmentRecord.Blockhash = pointer.String("bh")
		}
		require.NoError(t, e.data.PutAllFulfillments(e.ctx, newFulfillmentRecords...))
		fulfillmentRecords = append(fulfillmentRecords, newFulfillmentRecords...)
	}

	return fulfillmentRecords
}

func (e *schedulerTestEnv) simulatePrivacyUpgrades(t *testing.T, fulfillmentRecords []*fulfillment.Record) ([]*fulfillment.Record, *commitment.Record) {
	futureIntentId := testutil.NewRandomAccount(t).PublicKey().ToBase58()

	// The temp privacy action and fulfillment are confirmed for simplicity to scope
	// scheduling test code to the fulfillments it actually cares about when operating
	// on permanent upgraes.
	futureCommitmentRecord := &commitment.Record{
		DataVersion: splitter_token.DataVersion1,

		Address: "future-commitment",
		Vault:   "future-commitment-vault",

		Pool:       "test-pool",
		RecentRoot: "future-recent-root",
		Transcript: "transcript",

		Destination: "unknown-bucket-account",
		Amount:      kin.ToQuarks(1000),

		Intent:   futureIntentId,
		ActionId: 5,

		Owner: "unknown-owner",
	}
	require.NoError(t, e.data.SaveCommitment(e.ctx, futureCommitmentRecord))

	futureIntentRecord := &intent.Record{
		IntentId:   futureIntentId,
		IntentType: intent.ReceivePaymentsPrivately,

		InitiatorOwnerAccount: "someone-else",

		ReceivePaymentsPrivatelyMetadata: &intent.ReceivePaymentsPrivatelyMetadata{
			Source:   "unknown-incoming-account",
			Quantity: kin.ToQuarks(12345),

			UsdMarketValue: 12345,
		},

		State: intent.StatePending,
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, futureIntentRecord))

	quantity := kin.ToQuarks(10000)
	futureActionRecord := &action.Record{
		Intent:     futureIntentRecord.IntentId,
		IntentType: futureIntentRecord.IntentType,

		ActionId:   futureCommitmentRecord.ActionId,
		ActionType: action.PrivateTransfer,

		Source:      "unknown-incoming-account",
		Destination: &futureCommitmentRecord.Destination,
		Quantity:    &quantity,

		State: action.StateConfirmed,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, futureActionRecord))

	futureTempPrivacyFulfillmentRecord := &fulfillment.Record{
		Intent:     futureIntentRecord.IntentId,
		IntentType: futureIntentRecord.IntentType,

		ActionId:   futureCommitmentRecord.ActionId,
		ActionType: action.PrivateTransfer,

		FulfillmentType: fulfillment.TemporaryPrivacyTransferWithAuthority,

		Data:      []byte("data"),
		Signature: pointer.String(generateRandomSignature()),

		Nonce:     pointer.String(getRandomNonce()),
		Blockhash: pointer.String("bh"),

		Source:      "unknown-incoming-account",
		Destination: &futureCommitmentRecord.Vault,

		IntentOrderingIndex:      futureIntentRecord.Id,
		ActionOrderingIndex:      futureCommitmentRecord.ActionId,
		FulfillmentOrderingIndex: 2000,

		State: fulfillment.StateConfirmed,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, futureTempPrivacyFulfillmentRecord))

	var newFulfillmentRecords []*fulfillment.Record
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillment.TemporaryPrivacyTransferWithAuthority {
			upgradedTransfer := &fulfillment.Record{
				Intent:     fulfillmentRecord.Intent,
				IntentType: fulfillmentRecord.IntentType,

				ActionId:   fulfillmentRecord.ActionId,
				ActionType: fulfillmentRecord.ActionType,

				FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority,
				Data:            []byte("data"),
				Signature:       pointer.String(generateRandomSignature()),

				Nonce:     fulfillmentRecord.Nonce,
				Blockhash: fulfillmentRecord.Blockhash,

				Source:      fulfillmentRecord.Source,
				Destination: &futureCommitmentRecord.Vault,

				IntentOrderingIndex:      fulfillmentRecord.IntentOrderingIndex,
				ActionOrderingIndex:      fulfillmentRecord.ActionOrderingIndex,
				FulfillmentOrderingIndex: 1000,
			}

			newFulfillmentRecords = append(newFulfillmentRecords, upgradedTransfer)
			require.NoError(t, e.data.PutAllFulfillments(e.ctx, upgradedTransfer))

			previousCommitmentRecord, err := e.data.GetCommitmentByAction(e.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
			require.NoError(t, err)

			previousCommitmentRecord.RepaymentDivertedTo = &futureCommitmentRecord.Vault
			require.NoError(t, e.data.SaveCommitment(e.ctx, previousCommitmentRecord))
		}
	}

	return append(fulfillmentRecords, newFulfillmentRecords...), futureCommitmentRecord
}

func (e *schedulerTestEnv) simulateSavingTreasuryRecentRoot(t *testing.T, fulfillmentRecords []*fulfillment.Record) []*fulfillment.Record {
	intentId := testutil.NewRandomAccount(t).PublicKey().ToBase58()

	intentRecord := &intent.Record{
		IntentId:   intentId,
		IntentType: intent.SaveRecentRoot,

		SaveRecentRootMetadata: &intent.SaveRecentRootMetadata{
			TreasuryPool:           "test-pool-address",
			PreviousMostRecentRoot: "intermediary-recent-root",
		},

		InitiatorOwnerAccount: "code",

		State: intent.StatePending,
	}
	require.NoError(t, e.data.SaveIntent(e.ctx, intentRecord))

	actionRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   0,
		ActionType: action.SaveRecentRoot,

		Source: "test-pool-vault",

		State: action.StatePending,
	}
	require.NoError(t, e.data.PutAllActions(e.ctx, actionRecord))

	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   actionRecord.ActionId,
		ActionType: action.SaveRecentRoot,

		FulfillmentType: fulfillment.SaveRecentRoot,
		Data:            []byte("data"),
		Signature:       pointer.String(generateRandomSignature()),

		Nonce:     pointer.String(getRandomNonce()),
		Blockhash: pointer.String("bh"),

		Source: "test-pool-vault",

		IntentOrderingIndex:      intentRecord.Id,
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, fulfillmentRecord))

	return append(fulfillmentRecords, fulfillmentRecord)
}

func (e *schedulerTestEnv) simulateOpeningCommitmentVault(t *testing.T, commitmentRecord *commitment.Record, fulfillmentRecords []*fulfillment.Record) []*fulfillment.Record {
	intentRecord, err := e.data.GetIntent(e.ctx, commitmentRecord.Intent)
	require.NoError(t, err)

	newFulfillmentRecords := []*fulfillment.Record{
		{
			FulfillmentType: fulfillment.InitializeCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.UploadCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.VerifyCommitmentProof,
		},
		{
			FulfillmentType: fulfillment.OpenCommitmentVault,
		},
	}

	for i, fulfillmentRecord := range newFulfillmentRecords {
		fulfillmentRecord.Intent = intentRecord.IntentId
		fulfillmentRecord.IntentType = intentRecord.IntentType

		fulfillmentRecord.ActionId = commitmentRecord.ActionId
		fulfillmentRecord.ActionType = action.PrivateTransfer

		fulfillmentRecord.Data = []byte("data")
		fulfillmentRecord.Signature = pointer.String(generateRandomSignature())

		fulfillmentRecord.Nonce = pointer.String(getRandomNonce())
		fulfillmentRecord.Blockhash = pointer.String("bh")

		fulfillmentRecord.Source = commitmentRecord.Vault

		fulfillmentRecord.IntentOrderingIndex = 0
		fulfillmentRecord.ActionOrderingIndex = 0
		fulfillmentRecord.FulfillmentOrderingIndex = uint32(i)
	}
	require.NoError(t, e.data.PutAllFulfillments(e.ctx, newFulfillmentRecords...))

	commitmentRecord.State = commitment.StateOpening
	require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))

	return append(fulfillmentRecords, newFulfillmentRecords...)
}

func (e *schedulerTestEnv) simulateClosingCommitmentVault(t *testing.T, commitmentRecord *commitment.Record, fulfillmentRecords []*fulfillment.Record) []*fulfillment.Record {
	intentRecord, err := e.data.GetIntent(e.ctx, commitmentRecord.Intent)
	require.NoError(t, err)

	newFulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   commitmentRecord.ActionId,
		ActionType: action.PrivateTransfer,

		FulfillmentType: fulfillment.CloseCommitmentVault,
		Data:            []byte("data"),
		Signature:       pointer.String(generateRandomSignature()),

		Nonce:     pointer.String(getRandomNonce()),
		Blockhash: pointer.String("bh"),

		Source: commitmentRecord.Vault,

		IntentOrderingIndex:      ^uint64(0),
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,
	}

	require.NoError(t, e.data.PutAllFulfillments(e.ctx, newFulfillmentRecord))

	commitmentRecord.State = commitment.StateClosing
	require.NoError(t, e.data.SaveCommitment(e.ctx, commitmentRecord))

	return append(fulfillmentRecords, newFulfillmentRecord)
}

func (e *schedulerTestEnv) simulateMarkingAccountAsDormantFromAction(t *testing.T, intentId string, actionId uint32) {
	actionRecord, err := e.data.GetActionById(e.ctx, intentId, actionId)
	require.NoError(t, err)
	require.Equal(t, action.StateUnknown, actionRecord.State)
	require.Equal(t, action.CloseDormantAccount, actionRecord.ActionType)

	// The action becomes pending is sufficient
	actionRecord.State = action.StatePending
	require.NoError(t, e.data.UpdateAction(e.ctx, actionRecord))
}

func (e *schedulerTestEnv) simulateConfirmedFulfillment(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	// Create a mock transaction record
	txnRecord := &transaction.Record{
		Signature:         *fulfillmentRecord.Signature,
		Slot:              e.getNextSlot(),
		BlockTime:         time.Now(),
		ConfirmationState: transaction.ConfirmationFinalized,
	}
	require.NoError(t, e.data.SaveTransaction(e.ctx, txnRecord))

	handler, ok := e.handlersByType[fulfillmentRecord.FulfillmentType]
	require.True(t, ok)
	require.NoError(t, handler.OnSuccess(e.ctx, fulfillmentRecord, txnRecord))

	fulfillmentRecord.State = fulfillment.StateConfirmed
	require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
}

func (e *schedulerTestEnv) simulateFailedFulfillment(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	fulfillmentRecord.State = fulfillment.StateFailed
	require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
}

func (e *schedulerTestEnv) simulateRevokedFulfillment(t *testing.T, fulfillmentRecord *fulfillment.Record) {
	fulfillmentRecord.State = fulfillment.StateRevoked
	require.NoError(t, e.data.UpdateFulfillment(e.ctx, fulfillmentRecord))
}

func (e *schedulerTestEnv) assertSchedulingState(t *testing.T, fulfillmentRecord *fulfillment.Record, expected bool) {
	isScheduled, err := e.scheduler.CanSubmitToBlockchain(e.ctx, fulfillmentRecord)
	require.NoError(t, err)
	assert.Equal(t, expected, isScheduled)
}

func (e *schedulerTestEnv) assertSchedulingStateIfNotConfirmed(t *testing.T, fulfillmentRecord *fulfillment.Record, expected bool) {
	if fulfillmentRecord.State == fulfillment.StateConfirmed {
		return
	}

	isScheduled, err := e.scheduler.CanSubmitToBlockchain(e.ctx, fulfillmentRecord)
	require.NoError(t, err)
	assert.Equal(t, expected, isScheduled)
}

func (e *schedulerTestEnv) assertReservedTreasuryFunds(t *testing.T, expected uint64) {
	actual, err := e.data.GetUsedTreasuryPoolDeficitFromCommitments(e.ctx, "test-pool-address")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func (e *schedulerTestEnv) getNextSlot() uint64 {
	e.nextSlot += 1
	return e.nextSlot
}

func getBaseTestFulfillmentRecord(t *testing.T) *fulfillment.Record {
	return &fulfillment.Record{
		Intent:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		IntentType: intent.UnknownType,

		ActionId:   0,
		ActionType: action.UnknownType,

		FulfillmentType: fulfillment.UnknownType,
		Data:            []byte("data"),
		Signature:       pointer.String(generateRandomSignature()),

		Nonce:     pointer.String(getRandomNonce()),
		Blockhash: pointer.String("bh"),

		Source:      testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Destination: nil,

		IntentOrderingIndex:      0,
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		State: fulfillment.StateUnknown,
	}
}

func filterFulfillmentsByType(fulfillmentRecords []*fulfillment.Record, fulfillmentType fulfillment.Type) []*fulfillment.Record {
	var res []*fulfillment.Record
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.FulfillmentType == fulfillmentType {
			res = append(res, fulfillmentRecord)
		}
	}
	return res
}

func getRandomNonce() string {
	return fmt.Sprintf("nonce%d", rand.Int31())
}

func generateRandomSignature() string {
	return fmt.Sprintf("txn%d", rand.Uint64())
}

func getDigit(num, place uint64) uint64 {
	r := num % uint64(math.Pow(10, float64(place)))
	return r / uint64(math.Pow(10, float64(place-1)))
}

// Utility to view scheduling state for demonstrations
func (e *schedulerTestEnv) printScheduledFulfillments(t *testing.T, fulfillmentRecords []*fulfillment.Record) {
	var descriptions []string
	for _, fulfillmentRecord := range fulfillmentRecords {
		if fulfillmentRecord.State.IsTerminal() {
			continue
		}

		scheduled, err := e.scheduler.CanSubmitToBlockchain(e.ctx, fulfillmentRecord)
		require.NoError(t, err)

		actionRecord, err := e.data.GetActionById(e.ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		require.NoError(t, err)

		var description string
		switch fulfillmentRecord.FulfillmentType {
		case fulfillment.InitializeLockedTimelockAccount:
			description = fmt.Sprintf("create %s timelock account", fulfillmentRecord.Source)
		case fulfillment.NoPrivacyTransferWithAuthority:
			description = fmt.Sprintf("%s -> %s for %d kin using transfer with no privacy", fulfillmentRecord.Source, *fulfillmentRecord.Destination, kin.FromQuarks(*actionRecord.Quantity))
		case fulfillment.NoPrivacyWithdraw:
			description = fmt.Sprintf("%s -> %s for %d kin using withdraw with no privacy", fulfillmentRecord.Source, *fulfillmentRecord.Destination, kin.FromQuarks(*actionRecord.Quantity))
		case fulfillment.TemporaryPrivacyTransferWithAuthority:
			description = fmt.Sprintf("%s -> %s for %d kin with temporary privacy", fulfillmentRecord.Source, *fulfillmentRecord.Destination, kin.FromQuarks(*actionRecord.Quantity))
		case fulfillment.PermanentPrivacyTransferWithAuthority:
			description = fmt.Sprintf("%s -> %s for %d kin with permanent privacy", fulfillmentRecord.Source, *fulfillmentRecord.Destination, kin.FromQuarks(*actionRecord.Quantity))
		case fulfillment.TransferWithCommitment:
			description = fmt.Sprintf("treasury -> %s for %d kin", *fulfillmentRecord.Destination, kin.FromQuarks(*actionRecord.Quantity))
		case fulfillment.CloseEmptyTimelockAccount:
			description = fmt.Sprintf("close %s timelock account", fulfillmentRecord.Source)
		case fulfillment.CloseDormantTimelockAccount:
			description = fmt.Sprintf("close %s dormant timelock account and withdraw funds to %s", fulfillmentRecord.Source, *fulfillmentRecord.Destination)
		case fulfillment.SaveRecentRoot:
			description = "save block on the treasury"
		case fulfillment.InitializeCommitmentProof:
			description = fmt.Sprintf("initialize proof for %s", fulfillmentRecord.Source)
		case fulfillment.UploadCommitmentProof:
			description = fmt.Sprintf("upload proof [%d/3] for %s", fulfillmentRecord.FulfillmentOrderingIndex, fulfillmentRecord.Source)
		case fulfillment.VerifyCommitmentProof:
			description = fmt.Sprintf("verify proof for %s", fulfillmentRecord.Source)
		case fulfillment.OpenCommitmentVault:
			description = fmt.Sprintf("open commitment vault %s", fulfillmentRecord.Source)
		case fulfillment.CloseCommitmentVault:
			description = fmt.Sprintf("close commitment vault %s", fulfillmentRecord.Source)
		default:
			assert.Fail(t, "unsupported fulfillment type")
		}

		if scheduled {
			descriptions = append(descriptions, description)
		}
	}

	sort.Strings(descriptions)
	printForTest("scheduled fulfillments:")
	for _, description := range descriptions {
		printForTest("* %s", description)
	}
}

func printForTest(msg string, args ...any) {
	if !isVerbose {
		return
	}

	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	fmt.Printf(msg, args...)
}
