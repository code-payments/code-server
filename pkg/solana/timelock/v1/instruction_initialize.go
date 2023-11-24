package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var initializeInstructionDiscriminator = []byte{
	175, 175, 109, 31, 13, 152, 155, 237,
}

const (
	InitializeInstructionArgsSize = (1) // numDaysLocked

	InitializeInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // vaultOwner
		32 + // mint
		32 + // timeAuthority
		32 + // payer
		32 + // splTokenProgram
		32 + // systemProgram
		32) // sysvarRent

	InitializeInstructionSize = (8 + // discriminator
		InitializeInstructionArgsSize + // args
		InitializeInstructionAccountsSize) // accounts
)

type InitializeInstructionArgs struct {
	NumDaysLocked uint8
}

type InitializeInstructionAccounts struct {
	Timelock      ed25519.PublicKey
	Vault         ed25519.PublicKey
	VaultOwner    ed25519.PublicKey
	Mint          ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	Payer         ed25519.PublicKey
}

func NewInitializeInstruction(
	accounts *InitializeInstructionAccounts,
	args *InitializeInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(initializeInstructionDiscriminator)+
			InitializeInstructionArgsSize)

	putDiscriminator(data, initializeInstructionDiscriminator, &offset)
	putUint8(data, args.NumDaysLocked, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Timelock,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Vault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VaultOwner,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.TimeAuthority,
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

func InitializeInstructionFromBinary(data []byte) (*InitializeInstructionArgs, *InitializeInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < InitializeInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, initializeInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args InitializeInstructionArgs
	var accounts InitializeInstructionAccounts

	// Instruction Args
	getUint8(data, &args.NumDaysLocked, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.Mint, &offset)
	getKey(data, &accounts.TimeAuthority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
