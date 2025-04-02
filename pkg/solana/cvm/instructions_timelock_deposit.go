package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	DepositInstructionArgsSize = (2 + // account_index
		8 + //amount
		1) // bump
)

type DepositInstructionArgs struct {
	AccountIndex uint16
	Amount       uint64
	Bump         uint8
}

type DepositInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	Depositor   ed25519.PublicKey
	DepositPda  ed25519.PublicKey
	DepositAta  ed25519.PublicKey
	VmOmnibus   ed25519.PublicKey
}

func NewDepositInstruction(
	accounts *DepositInstructionAccounts,
	args *DepositInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+DepositInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionDeposit, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putUint64(data, args.Amount, &offset)
	putUint8(data, args.Bump, &offset)

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
				PublicKey:  accounts.Depositor,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.DepositPda,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.DepositAta,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VmOmnibus,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
