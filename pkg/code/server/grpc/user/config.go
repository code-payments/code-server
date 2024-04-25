package user

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "USER_SERVICE_"

	EnableBuyModuleConfigEnvName = envConfigPrefix + "ENABLE_BUY_MODULE"
	defaultEnableBuyModule       = true
)

type conf struct {
	enableBuyModule config.Bool
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			enableBuyModule: env.NewBoolConfig(EnableBuyModuleConfigEnvName, defaultEnableBuyModule),
		}
	}
}

type testOverrides struct {
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			enableBuyModule: wrapper.NewBoolConfig(memory.NewConfig(defaultEnableBuyModule), defaultEnableBuyModule),
		}
	}
}
