package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var revokeLockWithTimeoutInstructionDiscriminator = []byte{
	19, 131, 47, 77, 60, 188, 120, 124,
}

const (
	RevokeLockWithTimeoutInstructionArgsSize = (1) // TimelockBump

	RevokeLockWithTimeoutInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // vaultOwner
		32) // payer

	RevokeLockWithTimeoutInstructionSize = (8 + // discriminator
		RevokeLockWithTimeoutInstructionArgsSize + // args
		RevokeLockWithTimeoutInstructionAccountsSize) // accounts
)

type RevokeLockWithTimeoutInstructionArgs struct {
	TimelockBump uint8
}

type RevokeLockWithTimeoutInstructionAccounts struct {
	Timelock   ed25519.PublicKey
	Vault      ed25519.PublicKey
	VaultOwner ed25519.PublicKey
	Payer      ed25519.PublicKey
}

func NewRevokeLockWithTimeoutInstruction(
	accounts *RevokeLockWithTimeoutInstructionAccounts,
	args *RevokeLockWithTimeoutInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(revokeLockWithTimeoutInstructionDiscriminator)+
			RevokeLockWithTimeoutInstructionArgsSize)

	putDiscriminator(data, revokeLockWithTimeoutInstructionDiscriminator, &offset)
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
				PublicKey:  accounts.VaultOwner,
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

func RevokeLockWithTimeoutInstructionFromBinary(data []byte) (*RevokeLockWithTimeoutInstructionArgs, *RevokeLockWithTimeoutInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < RevokeLockWithTimeoutInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, revokeLockWithTimeoutInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args RevokeLockWithTimeoutInstructionArgs
	var accounts RevokeLockWithTimeoutInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
