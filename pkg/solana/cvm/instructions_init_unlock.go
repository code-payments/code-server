package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitUnlockInstructionArgsSize = 0
)

type InitUnlockInstructionArgs struct {
}

type InitUnlockInstructionAccounts struct {
	AccountOwner ed25519.PublicKey
	Payer        ed25519.PublicKey
	Vm           ed25519.PublicKey
	UnlockState  ed25519.PublicKey
}

func NewInitUnlockInstruction(
	accounts *InitUnlockInstructionAccounts,
	args *InitUnlockInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitUnlockInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitUnlock, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
			{
				PublicKey:  accounts.AccountOwner,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Vm,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.UnlockState,
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
