package antispam

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowOpenAccounts determines whether a phone-verified owner account can create
// a Code account via an open accounts intent.
func (g *Guard) AllowOpenAccounts(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowOpenAccounts")
	defer tracer.End()

	return true, nil
}

// AllowSendPayment determines whether a phone-verified owner account is allowed to
// make a send public/private payment intent.
func (g *Guard) AllowSendPayment(ctx context.Context, owner *common.Account, isPublic bool, destination *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSendPayment")
	defer tracer.End()

	return true, nil
}

// AllowReceivePayments determines whether a phone-verified owner account is allowed to
// make a public/private receive payments intent. The objective is to limit pressure on
// the scheduling layer.
func (g *Guard) AllowReceivePayments(ctx context.Context, owner *common.Account, isPublic bool) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowReceivePayments")
	defer tracer.End()

	return true, nil
}
