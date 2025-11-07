package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	CancelSwapInstructionArgsSize = (2 + // account_index
		8 + // amount
		1) // bump
)

type CancelSwapInstructionArgs struct {
	AccountIndex uint16
	Amount       uint64
	Bump         uint8
}

type CancelSwapInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	Swapper     ed25519.PublicKey
	SwapPda     ed25519.PublicKey
	SwapAta     ed25519.PublicKey
	VmOmnibus   ed25519.PublicKey
}

func NewCancelSwapInstruction(
	accounts *CancelSwapInstructionAccounts,
	args *CancelSwapInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+CancelSwapInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionCancelSwap, &offset)
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
				PublicKey:  accounts.Swapper,
				IsWritable: false,
				IsSigner:   false,
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
