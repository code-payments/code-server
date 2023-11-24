package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var initializePoolInstructionDiscriminator = []byte{
	95, 180, 10, 172, 84, 174, 232, 40,
}

const (
	InitializePoolInstructionArgsSize = (MaxNameLength + // Name
		1) // Levels

	InitializePoolInstructionAccountsSize = (32 + // pool
		32 + // vault
		32 + // mint
		32 + // authority
		32 + // payer
		32 + // splTokenProgram
		32 + // systemProgram
		32) // sysvarRent

	InitializePoolInstructionSize = (8 + // discriminator
		InitializePoolInstructionArgsSize + // args
		InitializePoolInstructionAccountsSize) // accounts
)

type InitializePoolInstructionArgs struct {
	Name   string
	Levels uint8
}

type InitializePoolInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Vault     ed25519.PublicKey
	Mint      ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewInitializePoolInstruction(
	accounts *InitializePoolInstructionAccounts,
	args *InitializePoolInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(initializePoolInstructionDiscriminator)+
			InitializePoolInstructionArgsSize)

	putDiscriminator(data, initializePoolInstructionDiscriminator, &offset)
	putName(data, args.Name, &offset)
	putUint8(data, args.Levels, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Pool,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Vault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Authority,
				IsWritable: false,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
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

func InitializePoolInstructionFromBinary(data []byte) (*InitializePoolInstructionArgs, *InitializePoolInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < InitializePoolInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, initializePoolInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args InitializePoolInstructionArgs
	var accounts InitializePoolInstructionAccounts

	// Instruction Args
	getName(data, &args.Name, &offset)
	getUint8(data, &args.Levels, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.Mint, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
