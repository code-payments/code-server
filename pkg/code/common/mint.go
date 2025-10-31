package common

import (
	"bytes"
	"context"
	"errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/code-server/pkg/code/config"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
	"github.com/code-payments/code-server/pkg/usdc"
	"github.com/code-payments/code-server/pkg/usdt"
)

var (
	CoreMintAccount, _    = NewAccountFromPublicKeyBytes(config.CoreMintPublicKeyBytes)
	CoreMintQuarksPerUnit = uint64(config.CoreMintQuarksPerUnit)
	CoreMintDecimals      = config.CoreMintDecimals
	CoreMintName          = config.CoreMintName
	CoreMintSymbol        = config.CoreMintSymbol

	ErrUnsupportedMint = errors.New("unsupported mint")

	jeffyMintAccount, _       = NewAccountFromPublicKeyString(config.JeffyMintPublicKey)
	knicksNightMintAccount, _ = NewAccountFromPublicKeyString(config.KnicksNightMintPublicKey)
)

func GetBackwardsCompatMint(protoMint *commonpb.SolanaAccountId) (*Account, error) {
	if protoMint == nil {
		return CoreMintAccount, nil
	}
	return NewAccountFromProto(protoMint)

}

func FromCoreMintQuarks(quarks uint64) uint64 {
	return quarks / CoreMintQuarksPerUnit
}

func ToCoreMintQuarks(units uint64) uint64 {
	return units * CoreMintQuarksPerUnit
}

func IsCoreMint(mint *Account) bool {
	return bytes.Equal(mint.PublicKey().ToBytes(), CoreMintAccount.PublicKey().ToBytes())
}

func IsCoreMintUsdStableCoin() bool {
	switch CoreMintAccount.PublicKey().ToBase58() {
	case usdc.Mint, usdt.Mint:
		return true
	default:
		return false
	}
}

func GetMintQuarksPerUnit(mint *Account) uint64 {
	if mint.PublicKey().ToBase58() == CoreMintAccount.PublicKey().ToBase58() {
		return CoreMintQuarksPerUnit
	}
	return currencycreator.DefaultMintQuarksPerUnit
}

func IsSupportedMint(ctx context.Context, data code_data.Provider, mintAccount *Account) (bool, error) {
	_, err := GetVmConfigForMint(ctx, data, mintAccount)
	if err == ErrUnsupportedMint {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
