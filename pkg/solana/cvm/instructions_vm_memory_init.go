package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var VmMemoryInitInstructionDiscriminator = []byte{
	0x05, 0xd3, 0xfb, 0x74, 0x39, 0xbc, 0xc1, 0xad,
}

const (
	VmMemoryInitInstructionArgsSize = (4 + MaxMemoryAccountNameLength) // name
)

type VmMemoryInitInstructionArgs struct {
	Name string
}

type VmMemoryInitInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
}

func NewVmMemoryInitInstruction(
	accounts *VmMemoryInitInstructionAccounts,
	args *VmMemoryInitInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(VmMemoryInitInstructionDiscriminator)+
			VmMemoryInitInstructionArgsSize)

	putDiscriminator(data, VmMemoryInitInstructionDiscriminator, &offset)
	putString(data, args.Name, &offset)

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
