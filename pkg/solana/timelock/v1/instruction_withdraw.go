package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var withdrawInstructionDiscriminator = []byte{
	183, 18, 70, 156, 148, 109, 161, 34,
}

const (
	WithdrawInstructionArgsSize = (1) // TimelockBump

	WithdrawInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // vaultOwner
		32 + // destination
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	WithdrawInstructionSize = (8 + // discriminator
		WithdrawInstructionArgsSize + // args
		WithdrawInstructionAccountsSize) // accounts
)

type WithdrawInstructionArgs struct {
	TimelockBump uint8
}

type WithdrawInstructionAccounts struct {
	Timelock    ed25519.PublicKey
	Vault       ed25519.PublicKey
	VaultOwner  ed25519.PublicKey
	Destination ed25519.PublicKey
	Payer       ed25519.PublicKey
}

func NewWithdrawInstruction(
	accounts *WithdrawInstructionAccounts,
	args *WithdrawInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(withdrawInstructionDiscriminator)+
			WithdrawInstructionArgsSize)

	putDiscriminator(data, withdrawInstructionDiscriminator, &offset)
	putUint8(data, args.TimelockBump, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Timelock,
				IsWritable: false,
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
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Destination,
				IsWritable: true,
				IsSigner:   false,
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

func WithdrawInstructionFromBinary(data []byte) (*WithdrawInstructionArgs, *WithdrawInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < WithdrawInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, withdrawInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args WithdrawInstructionArgs
	var accounts WithdrawInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.Destination, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
