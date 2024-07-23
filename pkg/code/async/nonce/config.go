package async_nonce

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
)

const (
	envConfigPrefix = "NONCE_SERVICE_"

	cvmPublicKeyConfigEnvName = envConfigPrefix + "CVM_PUBLIC_KEY"
	defaultCvmPublicKey       = "invalid" // Ensure something valid is set

	solanaMainnetNoncePubkeyPrefixConfigEnvName = envConfigPrefix + "SOLANA_MAINNET_NONCE_PUBKEY_PREFIX"
	defaultSolanaMainnetNoncePubkeyPrefix       = "non"

	solanaMainnetNoncePoolSizeConfigEnvName = envConfigPrefix + "SOLANA_MAINNET_NONCE_POOL_SIZE"
	defaultSolanaMainnetNoncePoolSize       = 1000
)

type conf struct {
	cvmPublicKey                   config.String
	solanaMainnetNoncePubkeyPrefix config.String
	solanaMainnetNoncePoolSize     config.Uint64
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			cvmPublicKey:                   env.NewStringConfig(cvmPublicKeyConfigEnvName, defaultCvmPublicKey),
			solanaMainnetNoncePubkeyPrefix: env.NewStringConfig(solanaMainnetNoncePubkeyPrefixConfigEnvName, defaultSolanaMainnetNoncePubkeyPrefix),
			solanaMainnetNoncePoolSize:     env.NewUint64Config(solanaMainnetNoncePoolSizeConfigEnvName, defaultSolanaMainnetNoncePoolSize),
		}
	}
}
