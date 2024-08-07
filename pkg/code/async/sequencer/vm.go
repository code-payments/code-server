package async_sequencer

import (
	"context"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
)

// This method can be safely called multiple times, since we know "deleted" accounts
// will never be reopened or uncompressed back into memory
func onVirtualAccountDeleted(ctx context.Context, data code_data.Provider, address string) error {
	err := data.FreeVmMemoryByAddress(ctx, address)
	if err == ram.ErrNotReserved {
		return nil
	}
	return err
}
