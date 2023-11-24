package async_commitment

import (
	"context"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

func markFulfillmentAsActivelyScheduled(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.Id == 0 {
		return nil
	}

	if !fulfillmentRecord.DisableActiveScheduling {
		return nil
	}

	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return nil
	}

	// Note: different than Save, since we don't have distributed locks
	return data.MarkFulfillmentAsActivelyScheduled(ctx, fulfillmentRecord.Id)
}
