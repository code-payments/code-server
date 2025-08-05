package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var (
	MintPrefix     = []byte("mint")
	CurrencyPrefix = []byte("currency")
	PoolPrefix     = []byte("pool")
	TreasuryPrefix = []byte("treasury")
	MetadataPrefix = []byte("metadata")
)

type GetMintAddressArgs struct {
	Authority ed25519.PublicKey
	Name      string
	Seed      ed25519.PublicKey
}

func GetMintAddress(args *GetMintAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		MintPrefix,
		args.Authority,
		[]byte(toFixedString(args.Name, MaxCurrencyConfigAccountNameLength)),
		args.Seed,
	)
}

type GetCurrencyAddressArgs struct {
	Mint ed25519.PublicKey
}

func GetCurrencyAddress(args *GetCurrencyAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CurrencyPrefix,
		args.Mint,
	)
}

type GetPoolAddressArgs struct {
	Currency ed25519.PublicKey
}

func GetPoolAddress(args *GetPoolAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		PoolPrefix,
		args.Currency,
	)
}

type GetVaultAddressArgs struct {
	Pool ed25519.PublicKey
	Mint ed25519.PublicKey
}

func GetVaultAddress(args *GetVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		TreasuryPrefix,
		args.Pool,
		args.Mint,
	)
}

type GetMetadataAddressArgs struct {
	Mint ed25519.PublicKey
}

func GetMetadataAddress(args *GetMetadataAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		METADATA_PROGRAM_ID,
		MetadataPrefix,
		METADATA_PROGRAM_ID,
		args.Mint,
	)
}
