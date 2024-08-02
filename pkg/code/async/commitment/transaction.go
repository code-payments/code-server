package async_commitment

import (
	"context"
	"math"

	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

func (p *service) injectCloseCommitmentFulfillment(ctx context.Context, commitmentRecord *commitment.Record) error {
	// Idempotency check to ensure we don't double up on fulfillments
	_, err := p.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.CloseCommitment, commitmentRecord.Intent, commitmentRecord.ActionId)
	if err == nil {
		return nil
	} else if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return err
	}

	intentRecord, err := p.data.GetIntent(ctx, commitmentRecord.Intent)
	if err != nil {
		return err
	}

	// Transaction is created on demand at time of scheduling
	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   commitmentRecord.ActionId,
		ActionType: action.PrivateTransfer,

		FulfillmentType: fulfillment.CloseCommitment,

		Source: commitmentRecord.Address,

		IntentOrderingIndex:      uint64(math.MaxInt64),
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		DisableActiveScheduling: false,

		State: fulfillment.StateUnknown,
	}
	return p.data.PutAllFulfillments(ctx, fulfillmentRecord)
}
