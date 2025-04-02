package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitNonceInstructionArgsSize = 2 // account_index
)

type InitNonceInstructionArgs struct {
	AccountIndex uint16
}

type InitNonceInstructionAccounts struct {
	VmAuthority         ed25519.PublicKey
	Vm                  ed25519.PublicKey
	VmMemory            ed25519.PublicKey
	VirtualAccountOwner ed25519.PublicKey
}

func NewInitNonceInstruction(
	accounts *InitNonceInstructionAccounts,
	args *InitNonceInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitNonceInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitNonce, &offset)
	putUint16(data, args.AccountIndex, &offset)

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
