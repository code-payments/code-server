package transaction_v2

import (
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
	"github.com/code-payments/code-server/pkg/config/memory"
	"github.com/code-payments/code-server/pkg/config/wrapper"
)

const (
	envConfigPrefix = "TRANSACTION_V2_SERVICE_"

	DisableSubmitIntentConfigEnvName = envConfigPrefix + "DISABLE_SUBMIT_INTENT"
	defaultDisableSubmitIntent       = false

	DisableBlockchainChecksConfigEnvName = envConfigPrefix + "DISABLE_BLOCKCHAIN_CHECKS"
	defaultDisableBlockchainChecks       = false

	SubmitIntentTimeoutConfigEnvName = envConfigPrefix + "SUBMIT_INTENT_TIMEOUT"
	defaultSubmitIntentTimeout       = 5 * time.Second

	ClientReceiveTimeoutConfigEnvName = envConfigPrefix + "CLIENT_RECEIVE_TIMEOUT"
	defaultClientReceiveTimeout       = time.Second

	FeeCollectorTokenPublicKeyConfigEnvName = envConfigPrefix + "FEE_COLLECTOR_TOKEN_PUBLIC_KEY"
	defaultFeeCollectorPublicKey            = "invalid" // Ensure something valid is set

	EnableAirdropsConfigEnvName = envConfigPrefix + "ENABLE_AIRDROPS"
	defaultEnableAirdrops       = false

	AirdropperOwnerPublicKeyEnvName = envConfigPrefix + "AIRDROPPER_OWNER_PUBLIC_KEY"
	defaultAirdropperOwnerPublicKey = "invalid" // Ensure something valid is set

	MaxAirdropUsdValueEnvName = envConfigPrefix + "MAX_AIRDROP_USD_VALUE"
	defaultMaxAirdropUsdValue = 1.0
)

type conf struct {
	disableSubmitIntent        config.Bool
	disableAntispamChecks      config.Bool // To avoid limits during testing
	disableAmlChecks           config.Bool // To avoid limits during testing
	disableBlockchainChecks    config.Bool
	submitIntentTimeout        config.Duration
	clientReceiveTimeout       config.Duration
	feeCollectorTokenPublicKey config.String
	enableAirdrops             config.Bool
	airdropperOwnerPublicKey   config.String
	maxAirdropUsdValue         config.Float64
	stripedLockParallelization config.Uint64
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			disableSubmitIntent:        env.NewBoolConfig(DisableSubmitIntentConfigEnvName, defaultDisableSubmitIntent),
			disableAntispamChecks:      wrapper.NewBoolConfig(memory.NewConfig(false), false),
			disableAmlChecks:           wrapper.NewBoolConfig(memory.NewConfig(false), false),
			disableBlockchainChecks:    env.NewBoolConfig(DisableBlockchainChecksConfigEnvName, defaultDisableBlockchainChecks),
			submitIntentTimeout:        env.NewDurationConfig(SubmitIntentTimeoutConfigEnvName, defaultSubmitIntentTimeout),
			clientReceiveTimeout:       env.NewDurationConfig(ClientReceiveTimeoutConfigEnvName, defaultClientReceiveTimeout),
			feeCollectorTokenPublicKey: env.NewStringConfig(FeeCollectorTokenPublicKeyConfigEnvName, defaultFeeCollectorPublicKey),
			enableAirdrops:             env.NewBoolConfig(EnableAirdropsConfigEnvName, defaultEnableAirdrops),
			airdropperOwnerPublicKey:   env.NewStringConfig(AirdropperOwnerPublicKeyEnvName, defaultAirdropperOwnerPublicKey),
			maxAirdropUsdValue:         env.NewFloat64Config(MaxAirdropUsdValueEnvName, defaultMaxAirdropUsdValue),
			stripedLockParallelization: wrapper.NewUint64Config(memory.NewConfig(8192), 8192),
		}
	}
}

type testOverrides struct {
	disableSubmitIntent        bool
	enableAntispamChecks       bool
	enableAmlChecks            bool
	enableAirdrops             bool
	clientReceiveTimeout       time.Duration
	feeCollectorTokenPublicKey string
}

func withManualTestOverrides(overrides *testOverrides) ConfigProvider {
	return func() *conf {
		return &conf{
			disableSubmitIntent:        wrapper.NewBoolConfig(memory.NewConfig(overrides.disableSubmitIntent), defaultDisableSubmitIntent),
			disableAntispamChecks:      wrapper.NewBoolConfig(memory.NewConfig(!overrides.enableAntispamChecks), false),
			disableAmlChecks:           wrapper.NewBoolConfig(memory.NewConfig(!overrides.enableAmlChecks), false),
			disableBlockchainChecks:    wrapper.NewBoolConfig(memory.NewConfig(true), true),
			submitIntentTimeout:        wrapper.NewDurationConfig(memory.NewConfig(defaultSubmitIntentTimeout), defaultSubmitIntentTimeout),
			clientReceiveTimeout:       wrapper.NewDurationConfig(memory.NewConfig(overrides.clientReceiveTimeout), defaultClientReceiveTimeout),
			feeCollectorTokenPublicKey: wrapper.NewStringConfig(memory.NewConfig(overrides.feeCollectorTokenPublicKey), defaultFeeCollectorPublicKey),
			enableAirdrops:             wrapper.NewBoolConfig(memory.NewConfig(overrides.enableAirdrops), false),
			airdropperOwnerPublicKey:   wrapper.NewStringConfig(memory.NewConfig(defaultAirdropperOwnerPublicKey), defaultAirdropperOwnerPublicKey),
			maxAirdropUsdValue:         wrapper.NewFloat64Config(memory.NewConfig(defaultMaxAirdropUsdValue), defaultMaxAirdropUsdValue),
			stripedLockParallelization: wrapper.NewUint64Config(memory.NewConfig(4), 4),
		}
	}
}
