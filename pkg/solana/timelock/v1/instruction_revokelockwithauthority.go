package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var revokeLockWithAuthorityInstructionDiscriminator = []byte{
	229, 181, 58, 242, 171, 8, 201, 144,
}

const (
	RevokeLockWithAuthorityInstructionArgsSize = (1) // TimelockBump

	RevokeLockWithAuthorityInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // timeAuthority
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	RevokeLockWithAuthorityInstructionSize = (8 + // discriminator
		RevokeLockWithAuthorityInstructionArgsSize + // args
		RevokeLockWithAuthorityInstructionAccountsSize) // accounts
)

type RevokeLockWithAuthorityInstructionArgs struct {
	TimelockBump uint8
}

type RevokeLockWithAuthorityInstructionAccounts struct {
	Timelock      ed25519.PublicKey
	Vault         ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	Payer         ed25519.PublicKey
}

func NewRevokeLockWithAuthorityInstruction(
	accounts *RevokeLockWithAuthorityInstructionAccounts,
	args *RevokeLockWithAuthorityInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(revokeLockWithAuthorityInstructionDiscriminator)+
			RevokeLockWithAuthorityInstructionArgsSize)

	putDiscriminator(data, revokeLockWithAuthorityInstructionDiscriminator, &offset)
	putUint8(data, args.TimelockBump, &offset)

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
		},
	}
}

func RevokeLockWithAuthorityInstructionFromBinary(data []byte) (*RevokeLockWithAuthorityInstructionArgs, *RevokeLockWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < RevokeLockWithAuthorityInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, revokeLockWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args RevokeLockWithAuthorityInstructionArgs
	var accounts RevokeLockWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.TimeAuthority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
