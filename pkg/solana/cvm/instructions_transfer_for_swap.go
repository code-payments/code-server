package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	TransferForSwapInstructionArgsSize = (8 + //amount
		1) // bump
)

type TransferForSwapInstructionArgs struct {
	Amount uint64
	Bump   uint8
}

type TransferForSwapInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	Swapper     ed25519.PublicKey
	SwapPda     ed25519.PublicKey
	SwapAta     ed25519.PublicKey
	Destination ed25519.PublicKey
}

func NewTransferForSwapInstruction(
	accounts *TransferForSwapInstructionAccounts,
	args *TransferForSwapInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+TransferForSwapInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionTransferForSwap, &offset)
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
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Swapper,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.SwapPda,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.SwapAta,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Destination,
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
