package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	CompressInstructionArgsSize = (2 + // account_index
		SignatureSize) // signature
)

type CompressInstructionArgs struct {
	AccountIndex uint16
	Signature    Signature
}

type CompressInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	VmStorage   ed25519.PublicKey
}

func NewCompressInstruction(
	accounts *CompressInstructionAccounts,
	args *CompressInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+CompressInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionCompress, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putSignature(data, args.Signature, &offset)

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
				PublicKey:  accounts.VmStorage,
				IsWritable: true,
				IsSigner:   false,
			},
		},
	}
}
