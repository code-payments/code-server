package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var activateInstructionDiscriminator = []byte{
	194, 203, 35, 100, 151, 55, 170, 82,
}

const (
	ActivateInstructionArgsSize = (1 + // TimelockBump
		8) // UnlockDuration

	ActivateInstructionAccountsSize = (32 + // timelock
		32 + // vaultOwner
		32) // payer

	ActivateInstructionSize = (8 + // discriminator
		ActivateInstructionArgsSize + // args
		ActivateInstructionAccountsSize) // accounts
)

type ActivateInstructionArgs struct {
	TimelockBump   uint8
	UnlockDuration uint64
}

type ActivateInstructionAccounts struct {
	Timelock   ed25519.PublicKey
	VaultOwner ed25519.PublicKey
	Payer      ed25519.PublicKey
}

func NewActivateInstruction(
	accounts *ActivateInstructionAccounts,
	args *ActivateInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(activateInstructionDiscriminator)+
			ActivateInstructionArgsSize)

	putDiscriminator(data, activateInstructionDiscriminator, &offset)
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

func ActivateInstructionFromBinary(data []byte) (*ActivateInstructionArgs, *ActivateInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < ActivateInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, activateInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args ActivateInstructionArgs
	var accounts ActivateInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
