package common

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/code/config"
	code_data "github.com/code-payments/code-server/pkg/code/data"
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
	Authority *Account
	Vm        *Account
	Omnibus   *Account
	Mint      *Account
}

func GetVmConfigForMint(ctx context.Context, data code_data.Provider, mint *Account) (*VmConfig, error) {
	switch mint.PublicKey().ToBase58() {
	case CoreMintAccount.PublicKey().ToBase58():
		return &VmConfig{
			Authority: GetSubsidizer(),
			Vm:        CodeVmAccount,
			Omnibus:   CodeVmOmnibusAccount,
			Mint:      CoreMintAccount,
		}, nil
	case jeffyMintAccount.PublicKey().ToBase58():
		vaultRecord, err := data.GetKey(ctx, jeffyAuthority.PublicKey().ToBase58())
		if err != nil {
			return nil, err
		}

		authorityAccount, err := NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
		if err != nil {
			return nil, err
		}

		return &VmConfig{
			Authority: authorityAccount,
			Vm:        jeffyVmAccount,
			Omnibus:   jeffyVmOmnibusAccount,
			Mint:      mint,
		}, nil
	default:
		return nil, errors.New("unsupported mint")
	}
}
