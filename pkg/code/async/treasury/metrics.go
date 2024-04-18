package async_treasury

import (
	"context"
	"time"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	treasuryFundCheckEventName       = "TreasuryFundPollingCheck"
	recentRootIntentCreatedEventName = "RecentRootIntentCreated"
	merkleTreeSyncedEventName        = "MerkleTreeSynced"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			treasuryPoolRecords, err := p.data.GetAllTreasuryPoolsByState(ctx, treasury.PoolStateAvailable)
			if err != nil {
				continue
			}

			for _, treasuryPoolRecord := range treasuryPoolRecords {
				total, used, err := estimateTreasuryPoolFundingLevels(ctx, p.data, treasuryPoolRecord)
				if err != nil {
					continue
				}
				recordTreasuryFundEvent(ctx, treasuryPoolRecord.Name, total, used)
			}

			delay = time.Second - time.Since(start)
		}
	}
}

func estimateTreasuryPoolFundingLevels(ctx context.Context, data code_data.Provider, record *treasury.Record) (total uint64, used uint64, err error) {
	total, err = data.GetTotalAvailableTreasuryPoolFunds(ctx, record.Vault)
	if err != nil {
		return 0, 0, err
	}

	used, err = data.GetUsedTreasuryPoolDeficitFromCommitments(ctx, record.Address)
	if err != nil {
		return 0, 0, err
	}

	return total, used, nil
}

func recordTreasuryFundEvent(ctx context.Context, name string, total, used uint64) {
	var available uint64
	if used < total {
		available = total - used
	}

	metrics.RecordEvent(ctx, treasuryFundCheckEventName, map[string]interface{}{
		"treasury":  name,
		"available": kin.FromQuarks(available),
		"used":      kin.FromQuarks(used),
	})
}

func recordRecentRootIntentCreatedEvent(ctx context.Context, treasuryPoolName string) {
	metrics.RecordEvent(ctx, recentRootIntentCreatedEventName, map[string]interface{}{
		"treasury": treasuryPoolName,
	})
}

func recordMerkleTreeSyncedEvent(ctx context.Context, treasuryPoolName string) {
	metrics.RecordEvent(ctx, merkleTreeSyncedEventName, map[string]interface{}{
		"treasury": treasuryPoolName,
	})
}
