package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var TimelockDepositFromAtaInstructionDiscriminator = []byte{
	0xb8, 0x7a, 0x27, 0x63, 0x50, 0xc9, 0xa7, 0xd1,
}

const (
	TimelockDepositFromAtaInstructionArgsSize = (2 + // account_index
		8) // amount
)

type TimelockDepositFromAtaInstructionArgs struct {
	AccountIndex uint16
	Amount       uint64
}

type TimelockDepositFromAtaInstructionAccounts struct {
	VmAuthority  ed25519.PublicKey
	Vm           ed25519.PublicKey
	VmMemory     ed25519.PublicKey
	VmOmnibus    ed25519.PublicKey
	DepositorAta ed25519.PublicKey
	Depositor    ed25519.PublicKey
}

func NewTimelockDepositFromAtaInstruction(
	accounts *TimelockDepositFromAtaInstructionAccounts,
	args *TimelockDepositFromAtaInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(TimelockDepositFromAtaInstructionDiscriminator)+
			TimelockDepositFromAtaInstructionArgsSize)

	putDiscriminator(data, TimelockDepositFromAtaInstructionDiscriminator, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putUint64(data, args.Amount, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
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
