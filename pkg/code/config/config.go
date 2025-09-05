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

	// Random value. Replace with real subsidizer public keys
	SubsidizerPublicKey = "84ydcM4Yp59W6aZP6eSaKiAMaKidNLfb5k318sT2pm14"

	// Random value. Replace with real VM public keys
	VmAccountPublicKey = "BVMGLfRgr3nVFCH5DuW6VR2kfSDxq4EFEopXfwCDpYzb"
	VmOmnibusPublicKey = "GNw1t85VH8b1CcwB5933KBC7PboDPJ5EcQdGynbfN1Pb"

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
