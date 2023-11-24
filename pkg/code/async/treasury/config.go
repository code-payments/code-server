package async_treasury

import (
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "TREASURY_SERVICE_"

	HideInCrowdPrivacyLevelConfigEnvName = envConfigPrefix + "HIDE_IN_CROWD_PRIVACY_LEVEL"
	defaultHideInCrowdPrivacylevel       = 10

	AdvanceCollectionTimeoutConfigEnvName = envConfigPrefix + "ADVANCE_COLLECTION_TIMEOUT"
	defaultAdvanceCollectionTimeout       = 24 * time.Hour
)

type conf struct {
	hideInCrowdPrivacyLevel  config.Uint64
	advanceCollectionTimeout config.Duration
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			hideInCrowdPrivacyLevel:  env.NewUint64Config(HideInCrowdPrivacyLevelConfigEnvName, defaultHideInCrowdPrivacylevel),
			advanceCollectionTimeout: env.NewDurationConfig(AdvanceCollectionTimeoutConfigEnvName, defaultAdvanceCollectionTimeout),
		}
	}
}

type testOverrides struct {
	hideInTheCrowdPrivacyLevel uint64
	advanceCollectionTimeout   time.Duration
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	if overrides.hideInTheCrowdPrivacyLevel == 0 {
		overrides.hideInTheCrowdPrivacyLevel = defaultHideInCrowdPrivacylevel
	}

	if overrides.advanceCollectionTimeout == 0 {
		overrides.advanceCollectionTimeout = defaultAdvanceCollectionTimeout
	}

	return func() *conf {
		return &conf{
			hideInCrowdPrivacyLevel:  wrapper.NewUint64Config(memory.NewConfig(overrides.hideInTheCrowdPrivacyLevel), defaultHideInCrowdPrivacylevel),
			advanceCollectionTimeout: wrapper.NewDurationConfig(memory.NewConfig(overrides.advanceCollectionTimeout), defaultAdvanceCollectionTimeout),
		}
	}
}
