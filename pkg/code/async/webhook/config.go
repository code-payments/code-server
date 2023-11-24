package async_webhook

import (
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "WEBHOOK_SERVICE_"

	WorkerBatchSizeConfigEnvName = envConfigPrefix + "WORKER_BATCH_SIZE"
	defaultWorkerBatchSize       = 250

	WebhookTimeoutConfigEnvName = envConfigPrefix + "WEBHOOK_TIMEOUT"
	defaultWebhookTimeout       = 3 * time.Second
)

type conf struct {
	workerBatchSize config.Uint64
	webhookTimeout  config.Duration
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			workerBatchSize: env.NewUint64Config(WorkerBatchSizeConfigEnvName, defaultWorkerBatchSize),
			webhookTimeout:  env.NewDurationConfig(WebhookTimeoutConfigEnvName, defaultWebhookTimeout),
		}
	}
}

type testOverrides struct {
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			workerBatchSize: wrapper.NewUint64Config(memory.NewConfig(defaultWorkerBatchSize), defaultWorkerBatchSize),
			webhookTimeout:  wrapper.NewDurationConfig(memory.NewConfig(defaultWebhookTimeout), defaultWebhookTimeout),
		}
	}
}
