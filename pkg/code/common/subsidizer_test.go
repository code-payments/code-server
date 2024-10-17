package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/pointer"
)

func TestEstimateUsedSubsidizerBalance(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	fulfillmentRecords := []*fulfillment.Record{
		// These records are included in fee calculation
		{IntentType: intent.SendPrivatePayment, ActionType: action.PrivateTransfer, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority, State: fulfillment.StatePending, Intent: "i1", Data: []byte("txn"), Nonce: pointer.String("n1"), Blockhash: pointer.String("bh1"), Signature: pointer.String("s1"), Source: "source", Destination: pointer.String("destination")},
		{IntentType: intent.ReceivePaymentsPrivately, ActionType: action.PrivateTransfer, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority, State: fulfillment.StatePending, Intent: "i2", Data: []byte("txn"), Nonce: pointer.String("n2"), Blockhash: pointer.String("bh2"), Signature: pointer.String("s2"), Source: "source", Destination: pointer.String("destination")},
		{IntentType: intent.OpenAccounts, ActionType: action.OpenAccount, FulfillmentType: fulfillment.InitializeLockedTimelockAccount, State: fulfillment.StatePending, Intent: "i3", Data: []byte("txn"), Nonce: pointer.String("n3"), Blockhash: pointer.String("bh3"), Signature: pointer.String("s3"), Source: "source"},

		// These records aren't included in fee calculation
		{IntentType: intent.SendPrivatePayment, ActionType: action.PrivateTransfer, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority, State: fulfillment.StateUnknown, Intent: "i4", Data: []byte("txn"), Nonce: pointer.String("n4"), Blockhash: pointer.String("bh4"), Signature: pointer.String("s4"), Source: "source", Destination: pointer.String("destination")},
		{IntentType: intent.SendPrivatePayment, ActionType: action.PrivateTransfer, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority, State: fulfillment.StateFailed, Intent: "i5", Data: []byte("txn"), Nonce: pointer.String("n5"), Blockhash: pointer.String("bh5"), Signature: pointer.String("s5"), Source: "source", Destination: pointer.String("destination")},
		{IntentType: intent.ReceivePaymentsPrivately, ActionType: action.PrivateTransfer, FulfillmentType: fulfillment.PermanentPrivacyTransferWithAuthority, State: fulfillment.StateRevoked, Intent: "i6", Data: []byte("txn"), Nonce: pointer.String("n6"), Blockhash: pointer.String("bh6"), Signature: pointer.String("s6"), Source: "source", Destination: pointer.String("destination")},
		{IntentType: intent.OpenAccounts, ActionType: action.OpenAccount, FulfillmentType: fulfillment.InitializeLockedTimelockAccount, State: fulfillment.StateConfirmed, Intent: "i7", Data: []byte("txn"), Nonce: pointer.String("n7"), Blockhash: pointer.String("bh7"), Signature: pointer.String("s7"), Source: "source"},
	}
	require.NoError(t, data.PutAllFulfillments(ctx, fulfillmentRecords...))

	nonceRecords := []*nonce.Record{
		// These records are included in fee calculation
		{Address: "n1", State: nonce.StateUnknown},
		{Address: "n2", State: nonce.StateUnknown},
		{Address: "n3", State: nonce.StateUnknown},

		// Theese records aren't included in fee calculation
		{Address: "n4", State: nonce.StateInvalid},
		{Address: "n5", State: nonce.StateReserved, Blockhash: "bh5"},
		{Address: "n6", State: nonce.StateReleased, Blockhash: "bh6"},
	}
	for _, nonceRecord := range nonceRecords {
		nonceRecord.Authority = "code"
		nonceRecord.Environment = nonce.EnvironmentSolana
		nonceRecord.EnvironmentInstance = nonce.EnvironmentInstanceSolanaMainnet
		nonceRecord.Purpose = nonce.PurposeClientTransaction
		require.NoError(t, data.SaveNonce(ctx, nonceRecord))
	}

	fees, err := EstimateUsedSubsidizerBalance(ctx, data)
	require.NoError(t, err)
	assert.EqualValues(
		t,
		3*lamportsPerCreateNonceAccount+2*lamportsByFulfillment[fulfillment.PermanentPrivacyTransferWithAuthority]+lamportsByFulfillment[fulfillment.InitializeLockedTimelockAccount],
		fees,
	)
}
