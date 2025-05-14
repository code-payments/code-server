package aml

import (
	"context"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "aml.guard"

	eventName = "AntiMoneyLaunderingGuardDenial"
)

func recordDenialEvent(ctx context.Context, reason string) {
	kvPairs := map[string]interface{}{
		"reason": reason,
		"count":  1,
	}
	metrics.RecordEvent(ctx, eventName, kvPairs)
}
