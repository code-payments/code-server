package transaction_v2

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	userIntentCreatedEventName            = "UserIntentCreated"
	submitIntentLatencyBreakdownEventName = "SubmitIntentLatencyBreakdown"
	criticalSubmitIntentFailure           = "CriticalSubmitIntentFailure"

	airdropEventName = "Airdrop"
)

func recordUserIntentCreatedEvent(ctx context.Context, intentRecord *intent.Record) {
	metrics.RecordEvent(ctx, userIntentCreatedEventName, map[string]interface{}{
		"id":   intentRecord.IntentId,
		"type": intentRecord.IntentType.String(),
		"mint": intentRecord.MintAccount,
	})
}

func recordSubmitIntentLatencyBreakdownEvent(ctx context.Context, section string, latency time.Duration, actionCount int, intentType string) {
	latencyInMs := latency / time.Millisecond
	metrics.RecordEvent(ctx, submitIntentLatencyBreakdownEventName, map[string]interface{}{
		"section":      section,
		"latency_ms":   int(latencyInMs),
		"action_count": actionCount,
		"intent_type":  intentType,
	})
}

func recordCriticalSubmitIntentFailure(ctx context.Context, intentRecord *intent.Record, err error) {
	kvs := map[string]interface{}{
		"error": err.Error(),
	}

	if intentRecord != nil {
		if len(intentRecord.IntentId) > 0 {
			kvs["intent_id"] = intentRecord.IntentId
		}
		if len(intentRecord.InitiatorOwnerAccount) > 0 {
			kvs["user_public_key"] = intentRecord.InitiatorOwnerAccount
		}
	}

	metrics.RecordEvent(ctx, criticalSubmitIntentFailure, kvs)
}

func recordAirdropEvent(ctx context.Context, owner *common.Account, airdropType AirdropType) {
	metrics.RecordEvent(ctx, airdropEventName, map[string]interface{}{
		"owner":        owner.PublicKey().ToBase58(),
		"airdrop_type": airdropType.String(),
	})
}
