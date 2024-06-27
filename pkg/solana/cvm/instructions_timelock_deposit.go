package cvm

import (
	"crypto/ed25519"
)

var TimelockDepositInstructionDiscriminator = []byte{
	0xe8, 0x19, 0x3f, 0x63, 0x5f, 0x15, 0xcc, 0xde,
}

const (
	TimelockDepositInstructionArgsSize = (2 + // account_index
		8) // amount
)

type TimelockDepositInstructionArgs struct {
	AccountIndex uint16
	Amount       uint64
}

type TimelockDepositInstructionAccounts struct {
	VmAuthority  ed25519.PublicKey
	Vm           ed25519.PublicKey
	VmMemory     ed25519.PublicKey
	VmOmnibus    ed25519.PublicKey
	DepositorAta ed25519.PublicKey
	Depositor    ed25519.PublicKey
}

func NewTimelockDepositInstruction(
	accounts *TimelockDepositInstructionAccounts,
	args *TimelockDepositInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(TimelockDepositInstructionDiscriminator)+
			TimelockDepositInstructionArgsSize)

	putDiscriminator(data, TimelockDepositInstructionDiscriminator, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putUint64(data, args.Amount, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.VmAuthority,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Vm,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VmMemory,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VmOmnibus,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.DepositorAta,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Depositor,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
