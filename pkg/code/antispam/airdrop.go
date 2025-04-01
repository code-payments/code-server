package antispam

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowWelcomeBonus determines whether a phone-verified owner account can receive
// the welcome bonus.
func (g *Guard) AllowWelcomeBonus(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowWelcomeBonus")
	defer tracer.End()

	return true, nil
}

// AllowReferralBonus determines whether a phone-verified owner account can receive
// a referral bonus.
func (g *Guard) AllowReferralBonus(
	ctx context.Context,
	referrerOwner,
	onboardedOwner,
	airdropperOwner *common.Account,
	quarksGivenByReferrer uint64,
	exchangedIn currency.Code,
	nativeAmount float64,
) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowReferralBonus")
	defer tracer.End()

	return true, nil
}
