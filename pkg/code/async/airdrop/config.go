package async_airdrop

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
)

// todo: setup configs

const (
	envConfigPrefix = "AIRDROP_SERVICE_"

	DisableAirdropsConfigEnvName = envConfigPrefix + "DISABLE_AIRDROPS"
	defaultDisableAirdrops       = false

	AirdropperOwnerConfigEnvName = envConfigPrefix + "AIRDROPPER_OWNER"
	defaultAirdropperOwner       = "invalid" // ensure something valid is set

	NonceMemoryAccountConfigEnvName = envConfigPrefix + "NONCE_MEMORY_ACCOUNT"
	defaultNonceMemoryAccount       = "invalid" // ensure something valid is set
)

type conf struct {
	disableAirdrops    config.Bool
	airdropperOwner    config.String
	nonceMemoryAccount config.String
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			disableAirdrops:    env.NewBoolConfig(DisableAirdropsConfigEnvName, defaultDisableAirdrops),
			airdropperOwner:    env.NewStringConfig(AirdropperOwnerConfigEnvName, defaultAirdropperOwner),
			nonceMemoryAccount: env.NewStringConfig(NonceMemoryAccountConfigEnvName, defaultNonceMemoryAccount),
		}
	}
}
