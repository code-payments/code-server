package async_sequencer

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
)

type mockScheduler struct {
	shouldSchedule bool
}

func (s *mockScheduler) CanSubmitToBlockchain(_ context.Context, _ *fulfillment.Record) (bool, error) {
	return s.shouldSchedule, nil
}

type mockFulfillmentHandler struct {
	isScheduled bool

	supportsOnDemandTxnCreation bool

	isRevoked              bool
	isNonceUsedWhenRevoked bool

	isRecoveredFromFailure bool

	successCallbackExecuted bool
	failureCallbackExecuted bool
}

func (h *mockFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	return h.isScheduled, nil
}

func (h *mockFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return h.supportsOnDemandTxnCreation
}

func (h *mockFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	if !h.supportsOnDemandTxnCreation {
		return nil, errors.New("not supported")
	}

	txn := solana.NewTransaction(common.GetSubsidizer().PublicKey().ToBytes(), memo.Instruction(selectedNonce.Account.PublicKey().ToBase58()))
	if err := txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes()); err != nil {
		return nil, err
	}

	return &txn, nil
}

func (h *mockFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, transactionRecord *transaction.Record) error {
	h.successCallbackExecuted = true
	return nil
}

func (h *mockFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, transactionRecord *transaction.Record) (recovered bool, err error) {
	h.failureCallbackExecuted = true
	return h.isRecoveredFromFailure, nil
}

func (h *mockFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	return h.isRevoked, h.isNonceUsedWhenRevoked, nil
}

type mockActionHandler struct {
	callbackExecuted         bool
	reportedFulfillmentState fulfillment.State
}

func (h *mockActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	h.callbackExecuted = true
	h.reportedFulfillmentState = newState
	return nil
}

type mockIntentHandler struct {
	callbackExecuted bool
}

func (h *mockIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	h.callbackExecuted = true
	return nil
}
