package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitVmInstructionArgsSize = (1 + // lock_duration
		1 + // vm_bump
		1) // vm_omnibus_bump
)

type InitVmInstructionArgs struct {
	LockDuration  uint8
	VmBump        uint8
	VmOmnibusBump uint8
}

type InitVmInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmOmnibus   ed25519.PublicKey
	Mint        ed25519.PublicKey
}

func NewInitVmInstruction(
	accounts *InitVmInstructionAccounts,
	args *InitVmInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitVmInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitVm, &offset)
	putUint8(data, args.LockDuration, &offset)
	putUint8(data, args.VmBump, &offset)
	putUint8(data, args.VmOmnibusBump, &offset)

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
				PublicKey:  accounts.VmOmnibus,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
				IsSigner:   false,
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
			{
				PublicKey:  SYSVAR_RENT_PUBKEY,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
