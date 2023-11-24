package data

import (
	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
)

const (
	FixerApiKeyConfigEnvName = "FIXER_API_KEY"
	defaultFixerApiKey       = ""
)

// todo: Add other data store configs here (eg. postgres, solana, etc).
type conf struct {
	fixerApiKey config.String
}

// ConfigProvider defines how config values are pulled
//
// todo: Possibly introduce an override config provider, for things like scripts.
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			fixerApiKey: env.NewStringConfig(FixerApiKeyConfigEnvName, defaultFixerApiKey),
		}
	}
}
