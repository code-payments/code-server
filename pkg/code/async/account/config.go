package async_account

// todo: setup configs

const (
	envConfigPrefix = "ACCOUNT_SERVICE_" //nolint:unused
)

type conf struct {
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{}
	}
}
