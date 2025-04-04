package config

import (
	"github.com/mr-tron/base58"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/usdc"
)

// todo: more things can be pulled into here to configure the open code protocol
// todo: make these environment configs

const (
	CoreMintPublicKeyString = usdc.Mint
	CoreMintQuarksPerUnit   = uint64(usdc.QuarksPerUsdc)
	CoreMintSymbol          = currency_lib.USDC
	CoreMintDecimals        = usdc.Decimals

	SubsidizerPublicKey = "cash11ndAmdKFEnG2wrQQ5Zqvr1kN9htxxLyoPLYFUV"

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
