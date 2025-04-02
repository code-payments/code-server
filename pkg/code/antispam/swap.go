package antispam

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowSwap determines whether a phone-verified owner account can perform a swap.
// The objective here is to limit attacks against our Swap Subsidizer's SOL balance.
func (g *Guard) AllowSwap(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSwap")
	defer tracer.End()

	return true, nil
}
