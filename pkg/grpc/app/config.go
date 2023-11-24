package app

import (
	"time"

	"github.com/spf13/viper"
)

// Config is the application specific configuration.
// It is passed to the App.Init function, and is optional.
type Config map[string]interface{}

// BaseConfig contains the base configuration for services, as well as the
// application itself.
type BaseConfig struct {
	LogLevel string `mapstructure:"log_level"`

	AppName string `mapstructure:"app_name"`

	ListenAddress         string `mapstructure:"listen_address"`
	InsecureListenAddress string `mapstructure:"insecure_listen_address"`
	DebugListenAddress    string `mapstructure:"debug_listen_address"`

	// TLSCertificate is an optional URL that specified a TLS certificate to be
	// used for the gRPC server.
	//
	// Currently only two supported URL schemes are supported: file, s3.
	// If no scheme is specified, file is used.
	TLSCertificate string `mapstructure:"tls_certificate"`
	// TLSKey is an optional URL that specifies a TLS Private Key to be used for the
	// gRPC server.
	//
	// Currently only two supported URL schemes are supported: file, s3.
	// If no scheme is specified, file is used.
	TLSKey string `mapstructure:"tls_private_key"`

	ShutdownGracePeriod time.Duration `mapstructure:"shutdown_grace_period"`

	EnablePprof  bool `mapstructure:"enable_pprof"`
	EnableExpvar bool `mapstructure:"enable_expvar"`

	// Ballast for improving Go GC performance. Note that capacity will be
	// limited to 50% of the total memory.
	// https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap/
	EnableBallast   bool    `mapstructure:"enable_ballast"`
	BallastCapacity float32 `mapstructure:"ballast_capacity"`

	// Periodically terminate the application when there's a memory leak
	EnableMemoryLeakCron   bool   `mapstructure:"enable_memory_leak_cron"`
	MemoryLeakCronSchedule string `mapstructure:"memory_leak_cron_schedule"`

	// Metrics configuration across many providers
	NewRelicLicenseKey string `mapstructure:"new_relic_license_key"`

	// Arbitrary configuration that the service can define / implement.
	//
	// Users should use mapstructure.Decode for ServiceConfig.
	AppConfig Config `mapstructure:"app"`
}

var defaultConfig = BaseConfig{
	LogLevel: "info",

	ListenAddress:         ":8085",
	InsecureListenAddress: "localhost:8086",
	DebugListenAddress:    ":8123",

	ShutdownGracePeriod: 30 * time.Second,

	EnablePprof:  true,
	EnableExpvar: true,

	EnableBallast:   true,
	BallastCapacity: 0.333,

	EnableMemoryLeakCron:   false,
	MemoryLeakCronSchedule: "0 5 * * *",
}

func init() {
	_ = viper.BindEnv("log_level", "LOG_LEVEL")

	_ = viper.BindEnv("app_name", "APP_NAME")

	_ = viper.BindEnv("listen_address", "LISTEN_ADDRESS")
	_ = viper.BindEnv("insecure_listen_address", "INSECURE_LISTEN_ADDRESS")
	_ = viper.BindEnv("debug_listen_address", "DEBUG_LISTEN_ADDRESS")

	_ = viper.BindEnv("tls_certificate", "TLS_CERTIFICATE")
	_ = viper.BindEnv("tls_private_key", "TLS_PRIVATE_KEY")

	_ = viper.BindEnv("shutdown_grace_period", "SHUTDOWN_GRACE_PERIOD")

	_ = viper.BindEnv("enable_pprof", "ENABLE_PPROF")
	_ = viper.BindEnv("enable_expvar", "ENABLE_EXPVAR")

	_ = viper.BindEnv("enable_ballast", "ENABLE_BALLAST")
	_ = viper.BindEnv("ballast_capacity", "BALLAST_CAPACITY")

	_ = viper.BindEnv("enable_memory_leak_cron", "ENABLE_MEMORY_LEAK_CRON")
	_ = viper.BindEnv("memory_leak_cron_schedule", "MEMORY_LEAK_CRON_SCHEDULE")

	_ = viper.BindEnv("new_relic_license_key", "NEW_RELIC_LICENSE_KEY")
}
