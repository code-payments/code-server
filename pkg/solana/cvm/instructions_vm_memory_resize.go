package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var VmMemoryResizeInstructionDiscriminator = []byte{
	0x6a, 0x40, 0x89, 0xc0, 0xdc, 0x48, 0xcd, 0x59,
}

const (
	VmMemoryResizeInstructionArgsSize = 4 // len
)

type VmMemoryResizeInstructionArgs struct {
	Len uint32
}

type VmMemoryResizeInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
}

func NewVmMemoryResizeInstruction(
	accounts *VmMemoryResizeInstructionAccounts,
	args *VmMemoryResizeInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(VmMemoryResizeInstructionDiscriminator)+
			VmMemoryResizeInstructionArgsSize)

	putDiscriminator(data, VmMemoryResizeInstructionDiscriminator, &offset)
	putUint32(data, args.Len, &offset)

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
				PublicKey:  SYSTEM_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSVAR_RENT_PUBKEY,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
