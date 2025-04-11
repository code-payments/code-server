package antispam

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/metrics"
)

type Guard struct {
	integration Integration
}

func NewGuard(integration Integration) *Guard {
	return &Guard{integration: integration}
}

func (g *Guard) AllowOpenAccounts(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowOpenAccounts")
	defer tracer.End()

	allow, reason, err := g.integration.AllowOpenAccounts(ctx, owner)
	if err != nil {
		return false, err
	}
	if !allow {
		recordDenialEvent(ctx, actionOpenAccounts, reason)
	}
	return allow, nil
}

func (g *Guard) AllowWelcomeBonus(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowWelcomeBonus")
	defer tracer.End()

	allow, reason, err := g.integration.AllowWelcomeBonus(ctx, owner)
	if err != nil {
		return false, err
	}
	if !allow {
		recordDenialEvent(ctx, actionWelcomeBonus, reason)
	}
	return allow, nil
}

func (g *Guard) AllowSendPayment(ctx context.Context, owner, destination *common.Account, isPublic bool) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSendPayment")
	defer tracer.End()

	allow, reason, err := g.integration.AllowSendPayment(ctx, owner, destination, isPublic)
	if err != nil {
		return false, err
	}
	if !allow {
		recordDenialEvent(ctx, actionSendPayment, reason)
	}
	return allow, nil
}

func (g *Guard) AllowReceivePayments(ctx context.Context, owner *common.Account, isPublic bool) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowReceivePayments")
	defer tracer.End()

	allow, reason, err := g.integration.AllowReceivePayments(ctx, owner, isPublic)
	if err != nil {
		return false, err
	}
	if !allow {
		recordDenialEvent(ctx, actionReceivePayments, reason)
	}
	return allow, nil
}

func (g *Guard) AllowSwap(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSwap")
	defer tracer.End()

	allow, reason, err := g.integration.AllowSwap(ctx, owner)
	if err != nil {
		return false, err
	}
	if !allow {
		recordDenialEvent(ctx, actionSwap, reason)
	}
	return allow, nil
}
