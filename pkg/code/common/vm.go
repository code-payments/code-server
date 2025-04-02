package common

import "github.com/code-payments/code-server/pkg/code/config"

var (
	// The well-known Code VM instance
	CodeVmAccount, _ = NewAccountFromPublicKeyString(config.VmAccountPublicKey)

	// The well-known Code VM instance omnibus account
	CodeVmOmnibusAccount, _ = NewAccountFromPublicKeyString(config.VmOmnibusPublicKey)
)
