package async_commitment

import (
	"context"
	"errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

// Every other state is currently managed after successful fulfillment submission

func markCommitmentAsClosing(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StateClosing {
		return nil
	}

	if commitmentRecord.State != commitment.StateOpen {
		return errors.New("commitment in invalid state")
	}

	commitmentRecord.State = commitment.StateClosing
	return data.SaveCommitment(ctx, commitmentRecord)
}

func markCommitmentReadyForGC(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.State == commitment.StateReadyToRemoveFromMerkleTree {
		return nil
	}

	if commitmentRecord.State != commitment.StateReadyToOpen && commitmentRecord.State != commitment.StateClosed {
		return errors.New("commitment in invalid state")
	}

	commitmentRecord.State = commitment.StateReadyToRemoveFromMerkleTree
	return data.SaveCommitment(ctx, commitmentRecord)
}

func markTreasuryAsRepaid(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if commitmentRecord.TreasuryRepaid {
		return nil
	}

	commitmentRecord.TreasuryRepaid = true
	return data.SaveCommitment(ctx, commitmentRecord)
}
