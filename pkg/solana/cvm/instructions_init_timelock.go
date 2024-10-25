package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitTimelockInstructionArgsSize = (2 + // account_index
		1 + // virtual_timelock_bump
		1 + // virtual_vault_bump
		1) // vm_unlock_pda_bump
)

type InitTimelockInstructionArgs struct {
	AccountIndex        uint16
	VirtualTimelockBump uint8
	VirtualVaultBump    uint8
	VmUnlockPdaBump     uint8
}

type InitTimelockInstructionAccounts struct {
	VmAuthority         ed25519.PublicKey
	Vm                  ed25519.PublicKey
	VmMemory            ed25519.PublicKey
	VirtualAccountOwner ed25519.PublicKey
}

func NewInitTimelockInstruction(
	accounts *InitTimelockInstructionAccounts,
	args *InitTimelockInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitTimelockInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitTimelock, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putUint8(data, args.VirtualTimelockBump, &offset)
	putUint8(data, args.VirtualVaultBump, &offset)
	putUint8(data, args.VmUnlockPdaBump, &offset)

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
				PublicKey:  accounts.VirtualAccountOwner,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
