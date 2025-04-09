package lawenforcement

import (
	"context"

	"github.com/sirupsen/logrus"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AntiMoneyLaunderingGuard gates money movement by applying rules on operations
// of interest to discourage money laundering through Code.
type AntiMoneyLaunderingGuard struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewAntiMoneyLaunderingGuard(data code_data.Provider) *AntiMoneyLaunderingGuard {
	return &AntiMoneyLaunderingGuard{
		log:  logrus.StandardLogger().WithField("type", "aml/guard"),
		data: data,
	}
}

// AllowMoneyMovement determines whether an intent that moves funds is allowed
// to be executed.
func (g *AntiMoneyLaunderingGuard) AllowMoneyMovement(ctx context.Context, intentRecord *intent.Record) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowMoneyMovement")
	defer tracer.End()

	// All movements of money are public, so no limits apply
	return true, nil
}
