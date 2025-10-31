package common

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/config"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

var (
	// The well-known Code VM instance
	CodeVmAccount, _ = NewAccountFromPublicKeyString(config.VmAccountPublicKey)

	// The well-known Code VM instance omnibus account
	CodeVmOmnibusAccount, _ = NewAccountFromPublicKeyString(config.VmOmnibusPublicKey)

	// todo: DB store to track VM per mint
	jeffyAuthority, _              = NewAccountFromPublicKeyString(config.JeffyAuthorityPublicKey)
	jeffyVmAccount, _              = NewAccountFromPublicKeyString(config.JeffyVmAccountPublicKey)
	jeffyVmOmnibusAccount, _       = NewAccountFromPublicKeyString(config.JeffyVmOmnibusPublicKey)
	knicksNightAuthority, _        = NewAccountFromPublicKeyString(config.KnicksNightAuthorityPublicKey)
	knicksNightVmAccount, _        = NewAccountFromPublicKeyString(config.KnicksNightVmAccountPublicKey)
	knicksNightVmOmnibusAccount, _ = NewAccountFromPublicKeyString(config.KnicksNightVmOmnibusPublicKey)
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
		if jeffyAuthority.PrivateKey() == nil {
			vaultRecord, err := data.GetKey(ctx, jeffyAuthority.PublicKey().ToBase58())
			if err != nil {
				return nil, err
			}

			jeffyAuthority, err = NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
			if err != nil {
				return nil, err
			}
		}

		return &VmConfig{
			Authority: jeffyAuthority,
			Vm:        jeffyVmAccount,
			Omnibus:   jeffyVmOmnibusAccount,
			Mint:      mint,
		}, nil
	case knicksNightMintAccount.PublicKey().ToBase58():
		if knicksNightAuthority.PrivateKey() == nil {
			vaultRecord, err := data.GetKey(ctx, knicksNightAuthority.PublicKey().ToBase58())
			if err != nil {
				return nil, err
			}

			knicksNightAuthority, err = NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
			if err != nil {
				return nil, err
			}
		}

		return &VmConfig{
			Authority: knicksNightAuthority,
			Vm:        knicksNightVmAccount,
			Omnibus:   knicksNightVmOmnibusAccount,
			Mint:      mint,
		}, nil
	default:
		return nil, ErrUnsupportedMint
	}
}
