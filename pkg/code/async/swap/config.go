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

	ClientTimeoutToFundConfigEnvName = envConfigPrefix + "CLIENT_TIMEOUT_TO_FUND"
	defaultClientTimeoutToFund       = 3 * time.Minute

	ClientTimeoutToSwapConfigEnvName = envConfigPrefix + "CLIENT_TIMEOUT_TO_SWAP"
	defaultClientTimeoutToSwap       = 5 * time.Minute
)

type conf struct {
	batchSize           config.Uint64
	clientTimeoutToFund config.Duration
	clientTimeoutToSwap config.Duration
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			batchSize:           env.NewUint64Config(BatchSizeConfigEnvName, defaultFulfillmentBatchSize),
			clientTimeoutToFund: env.NewDurationConfig(ClientTimeoutToFundConfigEnvName, defaultClientTimeoutToFund),
			clientTimeoutToSwap: env.NewDurationConfig(ClientTimeoutToSwapConfigEnvName, defaultClientTimeoutToSwap),
		}
	}
}
