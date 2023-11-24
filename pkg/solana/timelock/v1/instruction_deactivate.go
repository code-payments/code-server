package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var deactivateInstructionDiscriminator = []byte{
	44, 112, 33, 172, 113, 28, 142, 13,
}

const (
	DeactivateInstructionArgsSize = (1) // TimelockBump

	DeactivateInstructionAccountsSize = (32 + // timelock
		32 + // vaultOwner
		32) // payer

	DeactivateInstructionSize = (8 + // discriminator
		DeactivateInstructionArgsSize + // args
		DeactivateInstructionAccountsSize) // accounts
)

type DeactivateInstructionArgs struct {
	TimelockBump uint8
}

type DeactivateInstructionAccounts struct {
	Timelock   ed25519.PublicKey
	VaultOwner ed25519.PublicKey
	Payer      ed25519.PublicKey
}

func NewDeactivateInstruction(
	accounts *DeactivateInstructionAccounts,
	args *DeactivateInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(deactivateInstructionDiscriminator)+
			DeactivateInstructionArgsSize)

	putDiscriminator(data, deactivateInstructionDiscriminator, &offset)
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
				PublicKey:  accounts.VaultOwner,
				IsWritable: false,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
			},
		},
	}
}

func DeactivateInstructionFromBinary(data []byte) (*DeactivateInstructionArgs, *DeactivateInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < DeactivateInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, deactivateInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args DeactivateInstructionArgs
	var accounts DeactivateInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
