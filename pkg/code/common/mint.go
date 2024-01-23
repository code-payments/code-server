package common

import (
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/usdc"
)

var (
	KinMintAccount, _  = NewAccountFromPublicKeyBytes(kin.TokenMint)
	UsdcMintAccount, _ = NewAccountFromPublicKeyString(usdc.Mint)
)
