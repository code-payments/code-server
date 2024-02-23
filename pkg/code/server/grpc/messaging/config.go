package messaging

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "MESSAGING_SERVICE_"

	DisableBlockchainChecksConfigEnvName = envConfigPrefix + "DISABLE_BLOCKCHAIN_CHECKS"
	defaultDisableBlockchainChecks       = false

	MaxFeeBasisPointsConfigEnvName = envConfigPrefix + "MAX_FEE_BASIS_POINTS"
	defaultMaxFeeBasisPoints       = 5000
)

type conf struct {
	disableBlockchainChecks config.Bool
	maxFeeBasisPoints       config.Uint64
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			disableBlockchainChecks: env.NewBoolConfig(DisableBlockchainChecksConfigEnvName, defaultDisableBlockchainChecks),
			maxFeeBasisPoints:       env.NewUint64Config(MaxFeeBasisPointsConfigEnvName, defaultMaxFeeBasisPoints),
		}
	}
}

type testOverrides struct {
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			disableBlockchainChecks: wrapper.NewBoolConfig(memory.NewConfig(true), true),
			maxFeeBasisPoints:       wrapper.NewUint64Config(memory.NewConfig(defaultMaxFeeBasisPoints), defaultMaxFeeBasisPoints),
		}
	}
}
