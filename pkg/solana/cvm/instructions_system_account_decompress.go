package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var SystemAccountDecompressInstructionDiscriminator = []byte{
	0x63, 0x05, 0x07, 0x80, 0x20, 0x84, 0x86, 0x53,
}

const (
	MinSystemAccountDecompressInstructionArgsSize = (8 + // discriminator
		2 + // account_index
		4 + // len(packed_va)
		4 + // len(proof)
		SignatureSize) // signature
)

type SystemAccountDecompressInstructionArgs struct {
	AccountIndex uint16
	PackedVa     []uint8
	Proof        HashArray
	Signature    Signature
}

type SystemAccountDecompressInstructionAccounts struct {
	VmAuthority     ed25519.PublicKey
	Vm              ed25519.PublicKey
	VmMemory        ed25519.PublicKey
	VmStorage       ed25519.PublicKey
	UnlockPda       *ed25519.PublicKey
	WithdrawReceipt *ed25519.PublicKey
}

func NewSystemAccountDecompressInstruction(
	accounts *SystemAccountDecompressInstructionAccounts,
	args *SystemAccountDecompressInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(SystemAccountDecompressInstructionDiscriminator)+
			GetSystemAccountDecompressInstructionArgsSize(args))

	putDiscriminator(data, SystemAccountDecompressInstructionDiscriminator, &offset)
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

func GetSystemAccountDecompressInstructionArgsSize(args *SystemAccountDecompressInstructionArgs) int {
	return (MinSystemAccountDecompressInstructionArgsSize +
		len(args.PackedVa) + // packed_va
		HashSize*len(args.Proof)) // proof
}
