package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var transferWithAuthorityInstructionDiscriminator = []byte{
	68, 128, 222, 192, 129, 69, 71, 165,
}

const (
	TransferWithAuthorityInstructionArgsSize = (1 + // TimelockBump
		8) // amount

	TransferWithAuthorityInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // vaultOwner
		32 + // timeAuthority
		32 + // destination
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	TransferWithAuthorityInstructionSize = (8 + // discriminator
		TransferWithAuthorityInstructionArgsSize + // args
		TransferWithAuthorityInstructionAccountsSize) // accounts
)

type TransferWithAuthorityInstructionArgs struct {
	TimelockBump uint8
	Amount       uint64
}

type TransferWithAuthorityInstructionAccounts struct {
	Timelock      ed25519.PublicKey
	Vault         ed25519.PublicKey
	VaultOwner    ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	Destination   ed25519.PublicKey
	Payer         ed25519.PublicKey
}

func NewTransferWithAuthorityInstruction(
	accounts *TransferWithAuthorityInstructionAccounts,
	args *TransferWithAuthorityInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(transferWithAuthorityInstructionDiscriminator)+
			TransferWithAuthorityInstructionArgsSize)

	putDiscriminator(data, transferWithAuthorityInstructionDiscriminator, &offset)
	putUint8(data, args.TimelockBump, &offset)
	putUint64(data, args.Amount, &offset)

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
				PublicKey:  accounts.TimeAuthority,
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

func TransferWithAuthorityInstructionFromBinary(data []byte) (*TransferWithAuthorityInstructionArgs, *TransferWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < TransferWithAuthorityInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, transferWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args TransferWithAuthorityInstructionArgs
	var accounts TransferWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)
	getUint64(data, &args.Amount, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.TimeAuthority, &offset)
	getKey(data, &accounts.Destination, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
