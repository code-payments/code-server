package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var SystemAccountCompressInstructionDiscriminator = []byte{
	0x50, 0xc8, 0x0f, 0x6c, 0xb5, 0x1d, 0x7d, 0x9c,
}

const (
	SystemAccountCompressInstructionArgsSize = (2 + // account_index
		SignatureSize) // signature
)

type SystemAccountCompressInstructionArgs struct {
	AccountIndex uint16
	Signature    Signature
}

type SystemAccountCompressInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	VmStorage   ed25519.PublicKey
}

func NewSystemAccountCompressInstruction(
	accounts *SystemAccountCompressInstructionAccounts,
	args *SystemAccountCompressInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(SystemAccountCompressInstructionDiscriminator)+
			SystemAccountCompressInstructionArgsSize)

	putDiscriminator(data, SystemAccountCompressInstructionDiscriminator, &offset)
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
