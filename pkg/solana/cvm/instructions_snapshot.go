package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	RelaySaveRecentRootInstructionArgsSize = 0
)

type RelaySaveRecentRootInstructionArgs struct {
}

type RelaySaveRecentRootInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	Relay       ed25519.PublicKey
}

func NewRelaySaveRecentRootInstruction(
	accounts *RelaySaveRecentRootInstructionAccounts,
	args *RelaySaveRecentRootInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+RelaySaveRecentRootInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionSnapshot, &offset)

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
				PublicKey:  accounts.Relay,
				IsWritable: true,
				IsSigner:   false,
			},
		},
	}
}
