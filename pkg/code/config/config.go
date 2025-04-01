package config

import (
	"github.com/mr-tron/base58"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/usdc"
)

// todo: more things can be pulled into here to configure the open code protocol
// todo: make these environment configs

const (
	// Random values. Replace with real mint configuration
	CoreMintPublicKeyString = "DWYE8SQkpestTvpCxGNTCRjC2E9Kn6TCnu2SxkddrEEU"
	CoreMintQuarksPerUnit   = uint64(usdc.QuarksPerUsdc)
	CoreMintSymbol          = currency_lib.USDC
	CoreMintDecimals        = usdc.Decimals

	// Random value. Replace with real subsidizer public keys
	SubsidizerPublicKey = "84ydcM4Yp59W6aZP6eSaKiAMaKidNLfb5k318sT2pm14"

	// Random value. Replace with real VM public keys
	VmAccountPublicKey = "BVMGLfRgr3nVFCH5DuW6VR2kfSDxq4EFEopXfwCDpYzb"
	VmOmnibusPublicKey = "GNw1t85VH8b1CcwB5933KBC7PboDPJ5EcQdGynbfN1Pb"
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
