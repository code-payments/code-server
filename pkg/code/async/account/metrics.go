package async_account

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/config"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	giftCardWorkerEventName  = "GiftCardWorkerPollingCheck"
	swapRetryWorkerEventName = "SwapRetryWorkerPollingCheck"

	airdropperBalanceEventName = "AirdropperBalancePollingCheck"
)

func (p *service) metricsGaugeWorker(ctx context.Context) error {
	delay := time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			start := time.Now()

			p.recordBackupQueueStatusPollingEvent(ctx)
			p.recordAidropAccountBalance(ctx)

			delay = time.Second - time.Since(start)
		}
	}
}

func (p *service) recordBackupQueueStatusPollingEvent(ctx context.Context) {
	count, err := p.data.GetAccountInfoCountRequiringAutoReturnCheck(ctx)
	if err == nil {
		metrics.RecordEvent(ctx, giftCardWorkerEventName, map[string]interface{}{
			"queue_size": count,
		})
	}
}

func (p *service) recordAidropAccountBalance(ctx context.Context) {
	if p.airdropper == nil {
		return
	}

	quarks, err := balance.CalculateFromCache(ctx, p.data, p.airdropper.Vault)
	if err == nil {
		metrics.RecordEvent(ctx, airdropperBalanceEventName, map[string]interface{}{
			"owner":           p.airdropper.VaultOwner.PublicKey().ToBase58(),
			"quarks":          quarks,
			"quarks_per_unit": config.CoreMintQuarksPerUnit,
		})
	}
}
