package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	MinDecompressInstructionArgsSize = (8 + // discriminator
		2 + // account_index
		4 + // len(packed_va)
		4 + // len(proof)
		SignatureSize) // signature
)

type DecompressInstructionArgs struct {
	AccountIndex uint16
	PackedVa     []uint8
	Proof        HashArray
	Signature    Signature
}

type DecompressInstructionAccounts struct {
	VmAuthority     ed25519.PublicKey
	Vm              ed25519.PublicKey
	VmMemory        ed25519.PublicKey
	VmStorage       ed25519.PublicKey
	UnlockPda       *ed25519.PublicKey
	WithdrawReceipt *ed25519.PublicKey
}

func NewDecompressInstruction(
	accounts *DecompressInstructionAccounts,
	args *DecompressInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+GetDecompressInstructionArgsSize(args))

	putCodeInstruction(data, CodeInstructionDecompress, &offset)
	putUint16(data, args.AccountIndex, &offset)
	putUint8Array(data, args.PackedVa, &offset)
	putHashArray(data, args.Proof, &offset)
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
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.UnlockPda),
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.WithdrawReceipt),
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}

func GetDecompressInstructionArgsSize(args *DecompressInstructionArgs) int {
	return (MinDecompressInstructionArgsSize +
		len(args.PackedVa) + // packed_va
		HashSize*len(args.Proof)) // proof
}
