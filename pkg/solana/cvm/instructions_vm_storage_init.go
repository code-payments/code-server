package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	MaxStorageAccountNameLength = 32
)

var VmStorageInitInstructionDiscriminator = []byte{
	0x9d, 0xdf, 0x16, 0xd1, 0x0f, 0x24, 0xfe, 0x09,
}

const (
	VmStorageInitInstructionArgsSize = (4 + MaxStorageAccountNameLength + // name
		1) // levels
)

type VmStorageInitInstructionArgs struct {
	Name   string
	Levels uint8
}

type VmStorageInitInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	VmStorage   ed25519.PublicKey
}

func NewVmStorageInitInstruction(
	accounts *VmStorageInitInstructionAccounts,
	args *VmStorageInitInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(VmStorageInitInstructionDiscriminator)+
			VmStorageInitInstructionArgsSize)

	putDiscriminator(data, VmStorageInitInstructionDiscriminator, &offset)
	putString(data, args.Name, &offset)
	putUint8(data, args.Levels, &offset)

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
				PublicKey:  accounts.VmStorage,
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
