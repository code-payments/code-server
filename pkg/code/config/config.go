package config

import (
	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/usdc"
)

// todo: more things can be pulled into here to configure the open code protocol
// todo: make these environment configs

const (
	CoreMintPublicKeyString = usdc.Mint
	CoreMintQuarksPerUnit   = uint64(usdc.QuarksPerUsdc)
	CoreMintDecimals        = usdc.Decimals
	CoreMintName            = "USDC"
	CoreMintSymbol          = "USDC"
	CoreMintDescription     = "USDC is a regulated digital currency established in 2018 and issued by Circle, a regulated financial service business that follows strict US laws and standards to protect your money. Every USDC in existence is backed by a US dollar or cash equivalent asset that Circle has in its publicly visible reserves, with assurances issued monthly by a Big Four accounting firm."
	CoreMintImageUrl        = "https://pbs.twimg.com/profile_images/1916937910928211968/CKblfanr_400x400.png"

	SubsidizerPublicKey = "cash11ndAmdKFEnG2wrQQ5Zqvr1kN9htxxLyoPLYFUV"

	VmAccountPublicKey = "DHwPoAFFW4bTLBEdbFURvj5mJUUXCofPZwXXiZtRxRxw"
	VmOmnibusPublicKey = "DLBG6mt2NbzvVmhUq1sTqpccJH2cvmtZ9rUVUD7d97uv"

	// todo: DB store to track VM per mint
	JeffyMintPublicKey      = "52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj"
	JeffyAuthorityPublicKey = "jfy1btcfsjSn2WCqLVaxiEjp4zgmemGyRsdCPbPwnZV"
	JeffyVmAccountPublicKey = "Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta"
	JeffyVmOmnibusPublicKey = "CQ5jni8XTXEcMFXS1ytNyTVbJBZHtHCzEtjBPowB3MLD"
)

var (
	CoreMintPublicKeyBytes []byte
)

func init() {
	decoded, err := base58.Decode(CoreMintPublicKeyString)
	if err != nil {
		panic(err)
	}
	CoreMintPublicKeyBytes = decoded
}
