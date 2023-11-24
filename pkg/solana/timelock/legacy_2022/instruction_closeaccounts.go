package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var closeAccountsInstructionDiscriminator = []byte{
	171, 222, 94, 233, 34, 250, 202, 1,
}

const (
	CloseAccountsInstructionArgsSize = (1) // TimelockBump

	CloseAccountsInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // closeAuthority
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	CloseAccountsInstructionSize = (8 + // discriminator
		CloseAccountsInstructionArgsSize + // args
		CloseAccountsInstructionAccountsSize) // accounts
)

type CloseAccountsInstructionArgs struct {
	TimelockBump uint8
}

type CloseAccountsInstructionAccounts struct {
	Timelock       ed25519.PublicKey
	Vault          ed25519.PublicKey
	CloseAuthority ed25519.PublicKey
	Payer          ed25519.PublicKey
}

func NewCloseAccountsInstruction(
	accounts *CloseAccountsInstructionAccounts,
	args *CloseAccountsInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(closeAccountsInstructionDiscriminator)+
			CloseAccountsInstructionArgsSize)

	putDiscriminator(data, closeAccountsInstructionDiscriminator, &offset)
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
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.CloseAuthority,
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

func CloseAccountsInstructionFromBinary(data []byte) (*CloseAccountsInstructionArgs, *CloseAccountsInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < CloseAccountsInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeAccountsInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseAccountsInstructionArgs
	var accounts CloseAccountsInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.CloseAuthority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
