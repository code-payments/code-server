package async_swap

import (
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
)

const (
	envConfigPrefix = "SWAP_SERVICE_"

	BatchSizeConfigEnvName      = envConfigPrefix + "WORKER_BATCH_SIZE"
	defaultFulfillmentBatchSize = 100

	ClientFundingTimeoutConfigEnvName = envConfigPrefix + "CLIENT_FUNDING_TIMEOUT"
	defaultClientFundingTimeout       = 3 * time.Minute
)

type conf struct {
	batchSize            config.Uint64
	clientFundingTimeout config.Duration
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			batchSize:            env.NewUint64Config(BatchSizeConfigEnvName, defaultFulfillmentBatchSize),
			clientFundingTimeout: env.NewDurationConfig(ClientFundingTimeoutConfigEnvName, defaultClientFundingTimeout),
		}
	}
}
