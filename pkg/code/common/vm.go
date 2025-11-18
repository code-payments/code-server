package common

import (
	"context"

	"github.com/pkg/errors"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

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
	farmerCoinAuthority, _         = NewAccountFromPublicKeyString(config.FarmerCoinAuthorityPublicKey)
	farmerCoinVmAccount, _         = NewAccountFromPublicKeyString(config.FarmerCoinVmAccountPublicKey)
	farmerCoinVmOmnibusAccount, _  = NewAccountFromPublicKeyString(config.FarmerCoinVmOmnibusPublicKey)
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
	case farmerCoinMintAccount.PublicKey().ToBase58():
		if farmerCoinAuthority.PrivateKey() == nil {
			vaultRecord, err := data.GetKey(ctx, farmerCoinAuthority.PublicKey().ToBase58())
			if err != nil {
				return nil, err
			}

			farmerCoinAuthority, err = NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
			if err != nil {
				return nil, err
			}
		}

		return &VmConfig{
			Authority: farmerCoinAuthority,
			Vm:        farmerCoinVmAccount,
			Omnibus:   farmerCoinVmOmnibusAccount,
			Mint:      mint,
		}, nil
	default:
		return nil, ErrUnsupportedMint
	}
}

func GetVirtualTimelockAccountLocationInMemory(ctx context.Context, vmIndexerClient indexerpb.IndexerClient, vm, owner *Account) (*Account, uint16, error) {
	resp, err := vmIndexerClient.GetVirtualTimelockAccounts(ctx, &indexerpb.GetVirtualTimelockAccountsRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Owner:     &indexerpb.Address{Value: owner.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, 0, err
	} else if resp.Result != indexerpb.GetVirtualTimelockAccountsResponse_OK {
		return nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	if len(resp.Items) > 1 {
		return nil, 0, errors.New("multiple results returned")
	} else if resp.Items[0].Storage.GetMemory() == nil {
		return nil, 0, errors.New("account is compressed or hasn't been initialized")
	}

	protoMemory := resp.Items[0].Storage.GetMemory()
	memory, err := NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, 0, err
	}
	return memory, uint16(protoMemory.Index), nil
}
