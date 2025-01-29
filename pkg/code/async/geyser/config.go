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

	BackupExternalDepositWorkerCountConfigEnvName = envConfigPrefix + "BACKUP_EXTERNAL_DEPOSIT_WORKER_COUNT"
	defaultBackupExternalDepositWorkerCount       = 32

	BackupExternalDepositWorkerIntervalConfigEnvName = envConfigPrefix + "BACKUP_EXTERNAL_DEPOSIT_WORKER_INTERVAL"
	defaultBackupExternalDepositWorkerInterval       = 15 * time.Second

	MessagingFeeCollectorPublicKeyConfigEnvName = envConfigPrefix + "MESSAGING_FEE_COLLECTOR_PUBLIC_KEY"
	defaultMessagingFeeCollectorPublicKey       = "invalid" // ensure something valid is set

	SwapSubsidizerPublicKeyConfigEnvName = envConfigPrefix + "SWAP_SUBSIDIZER_PUBLIC_KEY"
	defaultSwapSubsidizerPublicKey       = "invalid" // ensure something valid is set

	BackupMessagingWorkerIntervalConfigEnvName = envConfigPrefix + "BACKUP_MESSAGING_WORKER_INTERVAL"
	defaultBackupMessagingWorkerInterval       = 15 * time.Minute // Decrease significantly once feature is live
)

type conf struct {
	grpcPluginEndpoint config.String

	programUpdateWorkerCount config.Uint64
	programUpdateQueueSize   config.Uint64

	backupExternalDepositWorkerCount    config.Uint64
	backupExternalDepositWorkerInterval config.Duration

	backupTimelockWorkerInterval config.Duration

	messagingFeeCollectorPublicKey config.String
	backupMessagingWorkerInterval  config.Duration

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

			backupExternalDepositWorkerCount:    env.NewUint64Config(BackupExternalDepositWorkerCountConfigEnvName, defaultBackupExternalDepositWorkerCount),
			backupExternalDepositWorkerInterval: env.NewDurationConfig(BackupExternalDepositWorkerIntervalConfigEnvName, defaultBackupExternalDepositWorkerInterval),

			backupTimelockWorkerInterval: env.NewDurationConfig(BackupTimelockWorkerIntervalConfigEnvName, defaultBackupTimelockWorkerInterval),

			messagingFeeCollectorPublicKey: env.NewStringConfig(MessagingFeeCollectorPublicKeyConfigEnvName, defaultMessagingFeeCollectorPublicKey),
			backupMessagingWorkerInterval:  env.NewDurationConfig(BackupMessagingWorkerIntervalConfigEnvName, defaultBackupMessagingWorkerInterval),

			swapSubsidizerPublicKey: env.NewStringConfig(SwapSubsidizerPublicKeyConfigEnvName, defaultSwapSubsidizerPublicKey),
		}
	}
}
