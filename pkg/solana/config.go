package solana

type Environment string

const (
	EnvironmentDev  Environment = "https://api.devnet.solana.com"
	EnvironmentTest Environment = "https://api.testnet.solana.com"
	EnvironmentProd Environment = "https://api.mainnet-beta.solana.com"
)
