package config

import (
	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/usdc"
)

// todo: more things can be pulled into here to configure the open code protocol
// todo: make these environment configs

const (
	// Random values. Replace with real mint configuration
	CoreMintPublicKeyString = "DWYE8SQkpestTvpCxGNTCRjC2E9Kn6TCnu2SxkddrEEU"
	CoreMintQuarksPerUnit   = uint64(usdc.QuarksPerUsdc)
	CoreMintDecimals        = usdc.Decimals
	CoreMintName            = "USDC"
	CoreMintSymbol          = "USDC"
	CoreMintDescription     = "USDC is a regulated digital currency established in 2018 and issued by Circle, a regulated financial service business that follows strict US laws and standards to protect your money. Every USDC in existence is backed by a US dollar or cash equivalent asset that Circle has in its publicly visible reserves, with assurances issued monthly by a Big Four accounting firm."
	CoreMintImageUrl        = "https://flipcash-currency-assets.s3.us-east-1.amazonaws.com/EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v/icon.png"

	// Random value. Replace with real subsidizer public keys
	SubsidizerPublicKey = "84ydcM4Yp59W6aZP6eSaKiAMaKidNLfb5k318sT2pm14"

	// Random value. Replace with real VM public keys
	VmAccountPublicKey = "BVMGLfRgr3nVFCH5DuW6VR2kfSDxq4EFEopXfwCDpYzb"
	VmOmnibusPublicKey = "GNw1t85VH8b1CcwB5933KBC7PboDPJ5EcQdGynbfN1Pb"

	// todo: DB store to track VM per mint
	JeffyMintPublicKey            = "52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj"
	JeffyAuthorityPublicKey       = "jfy1btcfsjSn2WCqLVaxiEjp4zgmemGyRsdCPbPwnZV"
	JeffyVmAccountPublicKey       = "Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta"
	JeffyVmOmnibusPublicKey       = "CQ5jni8XTXEcMFXS1ytNyTVbJBZHtHCzEtjBPowB3MLD"
	KnicksNightMintPublicKey      = "497Wy6cY9BjWBiaDHzJ7TcUZqF2gE1Qm7yXtSj1vSr5W"
	KnicksNightAuthorityPublicKey = "kn1VC4z3LUupQu41jCxx3PGNDmVyN3PswcAuvtyyHzS"
	KnicksNightVmAccountPublicKey = "9Du5GuKYT21ydLQ9KzUTWWQ7NKdwoXB15y4ypNnnpbJa"
	KnicksNightVmOmnibusPublicKey = "CqiUoainSQuqJNUxngLxemtGHcJR8tAueReduaL42kmK"
	FarmerCoinMintPublicKey       = "2o4PFbDZ73BihFraknfVTQeUtELKAeVUL4oa6bkrYU3A"
	FarmerCoinAuthorityPublicKey  = "Farms86uyc1wqeHemtxFeUnN3Cs28d4PVYKymXzbMy6T"
	FarmerCoinVmAccountPublicKey  = "5x9SP9a7dEGxK4xy8kurh8RC2fxvL1DSXhTCdcAMgpdb"
	FarmerCoinVmOmnibusPublicKey  = "ANHQ3psrtquyYS2sGbJ6tpmVxZ9Sxq21BTNjD2Rf9Uvj"
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
