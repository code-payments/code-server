package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitMemoryInstructionArgsSize = (4 + MaxMemoryAccountNameLength + // name
		1 + // layout
		1) // vm_memory_bump
)

type InitMemoryInstructionArgs struct {
	Name         string
	Layout       MemoryLayout
	VmMemoryBump uint8
}

type InitMemoryInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
}

func NewInitMemoryInstruction(
	accounts *InitMemoryInstructionAccounts,
	args *InitMemoryInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitMemoryInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitMemory, &offset)
	putFixedString(data, args.Name, MaxMemoryAccountNameLength, &offset)
	putMemoryLayout(data, args.Layout, &offset)
	putUint8(data, args.VmMemoryBump, &offset)

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
