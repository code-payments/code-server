package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	MaxStorageAccountNameLength = 32
)

const (
	InitStorageInstructionArgsSize = (MaxStorageAccountNameLength + // name
		1) // vm_storage_bump
)

type InitStorageInstructionArgs struct {
	Name          string
	VmStorageBump uint8
}

type InitStorageInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmStorage   ed25519.PublicKey
}

func NewInitStorageInstruction(
	accounts *InitStorageInstructionAccounts,
	args *InitStorageInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitStorageInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitStorage, &offset)
	putFixedString(data, args.Name, MaxStorageAccountNameLength, &offset)
	putUint8(data, args.VmStorageBump, &offset)

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
