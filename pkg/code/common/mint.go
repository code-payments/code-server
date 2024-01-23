package common

import (
	"github.com/code-payments/code-server/pkg/kin"
)

var (
	KinMintAccount, _ = NewAccountFromPublicKeyBytes(kin.TokenMint)
)
