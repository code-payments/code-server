package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var SystemTimelockInitInstructionDiscriminator = []byte{
	0x07, 0x0b, 0xf5, 0xc5, 0x68, 0xfe, 0xb7, 0xb6,
}

const (
	SystemTimelockInitInstructionArgsSize = (2 + // account_index
		1 + // virtual_timelock_bump
		1 + // virtual_vault_bump
		1) // vm_unlock_pda_bump
)

type SystemTimelockInitInstructionArgs struct {
	AccountIndex        uint16
	VirtualTimelockBump uint8
	VirtualVaultBump    uint8
	VmUnlockPdaBump     uint8
}

type SystemTimelockInitInstructionAccounts struct {
	VmAuthority         ed25519.PublicKey
	Vm                  ed25519.PublicKey
	VmMemory            ed25519.PublicKey
	VirtualAccountOwner ed25519.PublicKey
}

func NewSystemTimelockInitInstruction(
	accounts *SystemTimelockInitInstructionAccounts,
	args *SystemTimelockInitInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(SystemTimelockInitInstructionDiscriminator)+
			SystemTimelockInitInstructionArgsSize)

	putDiscriminator(data, SystemTimelockInitInstructionDiscriminator, &offset)
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
