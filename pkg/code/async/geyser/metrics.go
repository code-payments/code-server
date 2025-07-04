package async_geyser

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	subscriptionStatusEventName = "GeyserConsumerSubscriptionPollingCheck"
	eventWorkerStatusEventName  = "GeyserConsumerWorkerPollingCheck"
	eventQueueStatusEventName   = "GeyserConsumerQueuePollingCheck"
	backupWorkerStatusEventName = "GeyserBackupWorkerPollingCheck"
	backupQueueStatusEventName  = "GeyserBackupQueuePollingCheck"

	programUpdateWorkerName   = "ProgramUpdate"
	slotUpdateWorkerName      = "SlotUpdate"
	externalDepositWorkerName = "ExternalDeposit"
	timelockStateWorkerName   = "TimelockState"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			p.recordSubscriptionStatusPollingEvent(ctx)
			p.recordEventWorkerStatusPollingEvent(ctx)
			p.recordEventQueueStatusPollingEvent(ctx)
			p.recordBackupWorkerStatusPollingEvent(ctx)
			p.recordBackupQueueStatusPollingEvent(ctx)

			delay = time.Second - time.Since(start)
		}
	}
}

func (p *service) recordSubscriptionStatusPollingEvent(ctx context.Context) {
	currentFinalizedSlot, _ := p.data.GetBlockchainSlot(ctx, solana.CommitmentFinalized)

	p.metricStatusLock.RLock()
	defer p.metricStatusLock.RUnlock()

	metrics.RecordEvent(ctx, subscriptionStatusEventName, map[string]interface{}{
		"event_type": programUpdateWorkerName,
		"is_active":  p.programUpdateSubscriptionStatus,
	})

	var slotsBehind uint64
	if currentFinalizedSlot > p.highestObservedFinalizedSlot {
		slotsBehind = currentFinalizedSlot - p.highestObservedFinalizedSlot
	}

	kvPairs := map[string]interface{}{
		"event_type": slotUpdateWorkerName,
		"is_active":  p.slotUpdateSubscriptionStatus,
	}
	if p.highestObservedFinalizedSlot > 0 {
		kvPairs["highest_observed_slot"] = p.highestObservedFinalizedSlot
		if currentFinalizedSlot > 0 {
			kvPairs["slots_behind"] = slotsBehind
		}
	}
	metrics.RecordEvent(ctx, subscriptionStatusEventName, kvPairs)
}

func (p *service) recordEventWorkerStatusPollingEvent(ctx context.Context) {
	p.metricStatusLock.Lock()
	defer p.metricStatusLock.Unlock()

	var eventsProcessed int
	var numActive int
	for _, workerMetrics := range p.programUpdateWorkerMetrics {
		if workerMetrics.active {
			numActive += 1
		}
		eventsProcessed += workerMetrics.eventsProcessed
		workerMetrics.eventsProcessed = 0
	}

	metrics.RecordEvent(ctx, eventWorkerStatusEventName, map[string]interface{}{
		"event_type":       programUpdateWorkerName,
		"active_count":     numActive,
		"total_count":      len(p.programUpdateWorkerMetrics),
		"events_processed": eventsProcessed,
	})
}

func (p *service) recordEventQueueStatusPollingEvent(ctx context.Context) {
	metrics.RecordEvent(ctx, eventQueueStatusEventName, map[string]interface{}{
		"event_type":   programUpdateWorkerName,
		"current_size": len(p.programUpdatesChan),
		"max_size":     p.conf.programUpdateQueueSize.Get(ctx),
	})
}

func (p *service) recordBackupWorkerStatusPollingEvent(ctx context.Context) {
	p.metricStatusLock.Lock()
	defer p.metricStatusLock.Unlock()

	timelockMetrics := map[string]interface{}{
		"worker_type": timelockStateWorkerName,
		"is_active":   p.backupTimelockStateWorkerStatus,
	}
	if p.backupTimelockStateWorkerDuration != nil {
		inSeconds := *p.backupTimelockStateWorkerDuration / time.Second
		timelockMetrics["duration_s"] = int(inSeconds)
		p.backupTimelockStateWorkerDuration = nil
	}
	metrics.RecordEvent(ctx, backupWorkerStatusEventName, timelockMetrics)

	metrics.RecordEvent(ctx, backupWorkerStatusEventName, map[string]interface{}{
		"worker_type": externalDepositWorkerName,
		"is_active":   p.backupExternalDepositWorkerStatus,
	})
}

func (p *service) recordBackupQueueStatusPollingEvent(ctx context.Context) {
	count, err := p.data.GetAccountInfoCountRequiringDepositSync(ctx)
	if err != nil {
		return
	}

	metrics.RecordEvent(ctx, backupQueueStatusEventName, map[string]interface{}{
		"worker_type":  externalDepositWorkerName,
		"current_size": count,
	})
}
