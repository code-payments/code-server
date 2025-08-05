package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitializeCurrencyInstructionArgsSize = (MaxCurrencyConfigAccountNameLength + // name
		MaxCurrencyConfigAccountSymbolLength + // symbol
		32 + // seed
		8 + // max_supply
		1 + // decimal_places
		1 + // bump
		1 + // mint_bump
		5) // padding
)

type InitializeCurrencyInstructionArgs struct {
	Name          string
	Symbol        string
	Seed          ed25519.PublicKey
	MaxSupply     uint64
	DecimalPlaces uint8
	Bump          uint8
	MintBump      uint8
}

type InitializeCurrencyInstructionAccounts struct {
	Authority ed25519.PublicKey
	Creator   ed25519.PublicKey
	Mint      ed25519.PublicKey
	Currency  ed25519.PublicKey
	Metadata  ed25519.PublicKey
}

func NewInitializeCurrencyInstruction(
	accounts *InitializeCurrencyInstructionAccounts,
	args *InitializeCurrencyInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitializeCurrencyInstructionArgsSize)

	putInstructionType(data, InstructionTypeInitializeCurrency, &offset)
	putFixedString(data, args.Name, MaxCurrencyConfigAccountNameLength, &offset)
	putFixedString(data, args.Symbol, MaxCurrencyConfigAccountSymbolLength, &offset)
	putKey(data, args.Seed, &offset)
	putUint64(data, args.MaxSupply, &offset)
	putUint8(data, args.DecimalPlaces, &offset)
	putUint8(data, args.Bump, &offset)
	putUint8(data, args.MintBump, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
			{
				PublicKey:  accounts.Authority,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Creator,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Currency,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Metadata,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  METADATA_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSTEM_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSVAR_RENT_PUBKEY,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
