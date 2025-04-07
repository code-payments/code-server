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

	VmAccountPublicKey = "DHwPoAFFW4bTLBEdbFURvj5mJUUXCofPZwXXiZtRxRxw"
	VmOmnibusPublicKey = "DLBG6mt2NbzvVmhUq1sTqpccJH2cvmtZ9rUVUD7d97uv"
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
