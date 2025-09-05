package common

import (
	"errors"

	"github.com/code-payments/code-server/pkg/code/config"
)

var (
	// The well-known Code VM instance
	CodeVmAccount, _ = NewAccountFromPublicKeyString(config.VmAccountPublicKey)

	// The well-known Code VM instance omnibus account
	CodeVmOmnibusAccount, _ = NewAccountFromPublicKeyString(config.VmOmnibusPublicKey)

	// todo: DB store to track VM per mint
	jeffyAuthority, _        = NewAccountFromPublicKeyString(config.JeffyAuthorityPublicKey)
	jeffyVmAccount, _        = NewAccountFromPublicKeyString(config.JeffyVmAccountPublicKey)
	jeffyVmOmnibusAccount, _ = NewAccountFromPublicKeyString(config.JeffyVmOmnibusPublicKey)
)

type VmConfig struct {
	Vm        *Account
	Omnibus   *Account
	Authority *Account
}

func GetVmConfigForMint(mint *Account) (*VmConfig, error) {
	switch mint.PublicKey().ToBase58() {
	case CoreMintAccount.PublicKey().ToBase58():
		return &VmConfig{
			Vm:        CodeVmAccount,
			Omnibus:   CodeVmOmnibusAccount,
			Authority: GetSubsidizer(),
		}, nil
	case jeffyMintAccount.PublicKey().ToBase58():
		return &VmConfig{
			Vm:        jeffyVmAccount,
			Omnibus:   jeffyVmOmnibusAccount,
			Authority: jeffyAuthority,
		}, nil
	default:
		return nil, errors.New("unsupported mint")
	}
}
