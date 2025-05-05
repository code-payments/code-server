package async_geyser

import (
	"time"

	"github.com/code-payments/code-server/pkg/config"
	"github.com/code-payments/code-server/pkg/config/env"
)

const (
	envConfigPrefix = "GEYSER_CONSUMER_SERVICE_"

	GrpcPluginEndointConfigEnvName = envConfigPrefix + "GRPC_PLUGIN_ENDPOINT"
	defaultGrpcPluginEndoint       = ""

	ProgramUpdateWorkerCountConfigEnvName = envConfigPrefix + "PROGRAM_UPDATE_WORKER_COUNT"
	defaultProgramUpdateWorkerCount       = 1024

	ProgramUpdateQueueSizeConfigEnvName = envConfigPrefix + "PROGRAM_UPDATE_QUEUE_SIZE"
	defaultProgramUpdateQueueSize       = 1_000_000

	BackupTimelockWorkerIntervalConfigEnvName = envConfigPrefix + "BACKUP_TIMELOCK_WORKER_INTERVAL"
	defaultBackupTimelockWorkerInterval       = 1 * time.Minute

	BackupExternalDepositWorkerIntervalConfigEnvName = envConfigPrefix + "BACKUP_EXTERNAL_DEPOSIT_WORKER_INTERVAL"
	defaultBackupExternalDepositWorkerInterval       = 15 * time.Second

	SwapSubsidizerPublicKeyConfigEnvName = envConfigPrefix + "SWAP_SUBSIDIZER_PUBLIC_KEY"
	defaultSwapSubsidizerPublicKey       = "invalid" // ensure something valid is set
)

type conf struct {
	grpcPluginEndpoint config.String

	programUpdateWorkerCount config.Uint64
	programUpdateQueueSize   config.Uint64

	backupExternalDepositWorkerInterval config.Duration

	backupTimelockWorkerInterval config.Duration

	swapSubsidizerPublicKey config.String
}

// ConfigProvider defines how config values are pulled
type ConfigProvider func() *conf

// WithEnvConfigs returns configuration pulled from environment variables
func WithEnvConfigs() ConfigProvider {
	return func() *conf {
		return &conf{
			grpcPluginEndpoint: env.NewStringConfig(GrpcPluginEndointConfigEnvName, defaultGrpcPluginEndoint),

			programUpdateWorkerCount: env.NewUint64Config(ProgramUpdateWorkerCountConfigEnvName, defaultProgramUpdateWorkerCount),
			programUpdateQueueSize:   env.NewUint64Config(ProgramUpdateQueueSizeConfigEnvName, defaultProgramUpdateQueueSize),

			backupExternalDepositWorkerInterval: env.NewDurationConfig(BackupExternalDepositWorkerIntervalConfigEnvName, defaultBackupExternalDepositWorkerInterval),

			backupTimelockWorkerInterval: env.NewDurationConfig(BackupTimelockWorkerIntervalConfigEnvName, defaultBackupTimelockWorkerInterval),

			swapSubsidizerPublicKey: env.NewStringConfig(SwapSubsidizerPublicKeyConfigEnvName, defaultSwapSubsidizerPublicKey),
		}
	}
}
