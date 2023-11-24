package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var cancelLockTimeoutInstructionDiscriminator = []byte{
	207, 206, 169, 241, 34, 199, 123, 249,
}

const (
	CancelLockTimeoutInstructionArgsSize = (1) // TimelockBump

	CancelLockTimeoutInstructionAccountsSize = (32 + // timelock
		32 + // timeAuthority
		32 + // payer
		32) // systemProgram

	CancelLockTimeoutInstructionSize = (8 + // discriminator
		CancelLockTimeoutInstructionArgsSize + // args
		CancelLockTimeoutInstructionAccountsSize) // accounts
)

type CancelLockTimeoutInstructionArgs struct {
	TimelockBump uint8
}

type CancelLockTimeoutInstructionAccounts struct {
	Timelock      ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	Payer         ed25519.PublicKey
}

func NewCancelLockTimeoutInstruction(
	accounts *CancelLockTimeoutInstructionAccounts,
	args *CancelLockTimeoutInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(cancelLockTimeoutInstructionDiscriminator)+
			CancelLockTimeoutInstructionArgsSize)

	putDiscriminator(data, cancelLockTimeoutInstructionDiscriminator, &offset)
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
				PublicKey:  SYSTEM_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}

func CancelLockTimeoutInstructionFromBinary(data []byte) (*CancelLockTimeoutInstructionArgs, *CancelLockTimeoutInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < CancelLockTimeoutInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, cancelLockTimeoutInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CancelLockTimeoutInstructionArgs
	var accounts CancelLockTimeoutInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.TimeAuthority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
