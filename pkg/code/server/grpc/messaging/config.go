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
)

type conf struct {
	disableBlockchainChecks config.Bool
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			disableBlockchainChecks: env.NewBoolConfig(DisableBlockchainChecksConfigEnvName, defaultDisableBlockchainChecks),
		}
	}
}

type testOverrides struct {
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			disableBlockchainChecks: wrapper.NewBoolConfig(memory.NewConfig(true), true),
		}
	}
}
