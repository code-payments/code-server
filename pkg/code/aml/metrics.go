package aml

import (
	"context"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "aml.guard"

	eventName = "AntiMoneyLaunderingGuardDenial"

	actionSendPayment = "SendPayment"
)

func recordDenialEvent(ctx context.Context, action, reason string) {
	kvPairs := map[string]interface{}{
		"action": action,
		"reason": reason,
		"count":  1,
	}
	metrics.RecordEvent(ctx, eventName, kvPairs)
}
