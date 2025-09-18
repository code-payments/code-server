package async_sequencer

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "SEQUENCER_SERVICE_"

	DisableTransactionSchedulingConfigEnvName = envConfigPrefix + "DISABLE_TRANSACTION_SCHEDULING"
	defaultDisableTransactionScheduling       = false

	DisableTransactionSubmissionConfigEnvName = envConfigPrefix + "DISABLE_TRANSACTION_SUBMISSION"
	defaultDisableTransactionSubmission       = false

	MaxGlobalFailedFulfillmentsConfigEnvName = envConfigPrefix + "MAX_GLOBAL_FAILED_FULFILLMENTS"
	defaultMaxGlobalFailedFulfillments       = 10

	FulfillmentBatchSizeConfigEnvName = envConfigPrefix + "WORKER_BATCH_SIZE"
	defaultFulfillmentBatchSize       = 100

	EnableSubsidizerChecksConfigEnvName = envConfigPrefix + "ENABLE_SUBSIDIZER_CHECKS"
	defaultEnableSubsidizerChecks       = true
)

type conf struct {
	disableTransactionScheduling  config.Bool
	disableTransactionSubmission  config.Bool
	maxGlobalFailedFulfillments   config.Uint64
	fulfillmentBatchSize          config.Uint64
	enableSubsidizerChecks        config.Bool
	enableCachedTransactionLookup config.Bool
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			disableTransactionScheduling:  env.NewBoolConfig(DisableTransactionSchedulingConfigEnvName, defaultDisableTransactionScheduling),
			disableTransactionSubmission:  env.NewBoolConfig(DisableTransactionSubmissionConfigEnvName, defaultDisableTransactionSubmission),
			maxGlobalFailedFulfillments:   env.NewUint64Config(MaxGlobalFailedFulfillmentsConfigEnvName, defaultMaxGlobalFailedFulfillments),
			fulfillmentBatchSize:          env.NewUint64Config(FulfillmentBatchSizeConfigEnvName, defaultFulfillmentBatchSize),
			enableSubsidizerChecks:        env.NewBoolConfig(EnableSubsidizerChecksConfigEnvName, defaultEnableSubsidizerChecks),
			enableCachedTransactionLookup: wrapper.NewBoolConfig(memory.NewConfig(false), false),
		}
	}
}

type testOverrides struct {
	disableTransactionScheduling bool
	maxGlobalFailedFulfillments  uint64
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			disableTransactionScheduling:  wrapper.NewBoolConfig(memory.NewConfig(overrides.disableTransactionScheduling), defaultDisableTransactionScheduling),
			disableTransactionSubmission:  wrapper.NewBoolConfig(memory.NewConfig(true), defaultDisableTransactionSubmission),
			maxGlobalFailedFulfillments:   wrapper.NewUint64Config(memory.NewConfig(overrides.maxGlobalFailedFulfillments), defaultMaxGlobalFailedFulfillments),
			fulfillmentBatchSize:          wrapper.NewUint64Config(memory.NewConfig(defaultFulfillmentBatchSize), defaultFulfillmentBatchSize),
			enableSubsidizerChecks:        wrapper.NewBoolConfig(memory.NewConfig(false), defaultEnableSubsidizerChecks),
			enableCachedTransactionLookup: wrapper.NewBoolConfig(memory.NewConfig(true), true),
		}
	}
}
