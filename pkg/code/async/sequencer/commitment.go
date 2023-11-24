package async_sequencer

import (
	"context"
	"errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

// todo: commitment state is highly correlated with fulfillment submission, so we manage
//       it here, at least for now

func markCommitmentPayingDestination(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StatePayingDestination {
		return nil
	}

	if commitmentRecord.State != commitment.StateUnknown {
		return errors.New("commitment in invalid state")
	}

	commitmentRecord.State = commitment.StatePayingDestination
	return data.SaveCommitment(ctx, commitmentRecord)
}

func markCommitmentReadyToOpen(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StateReadyToOpen {
		return nil
	}

	if commitmentRecord.State != commitment.StatePayingDestination {
		return errors.New("commitment in invalid state")
	}

	commitmentRecord.State = commitment.StateReadyToOpen
	return data.SaveCommitment(ctx, commitmentRecord)
}

// Opening is something managed externally

func markCommitmentOpen(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StateOpen {
		return nil
	}

	if commitmentRecord.State != commitment.StateOpening {
		return errors.New("commitment in invalid state")
	}

	fulfillmentRecords, err := data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, intentId, actionId)
	if err != nil {
		return err
	}
	err = markFulfillmentAsActivelyScheduled(ctx, data, fulfillmentRecords[0])
	if err != nil {
		return err
	}

	commitmentRecord.State = commitment.StateOpen
	return data.SaveCommitment(ctx, commitmentRecord)
}

// Closing is likely something managed externally

func markCommitmentClosed(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StateClosed {
		return nil
	}

	if commitmentRecord.State != commitment.StateClosing {
		return errors.New("commitment in invalid state")
	}

	commitmentRecord.State = commitment.StateClosed
	return data.SaveCommitment(ctx, commitmentRecord)
}
