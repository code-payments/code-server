package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var SystemNonceInitInstructionDiscriminator = []byte{
	0x0d, 0xb2, 0x98, 0x60, 0xa7, 0x2c, 0x96, 0x30,
}

const (
	SystemNonceInitInstructionArgsSize = 2 // account_index
)

type SystemNonceInitInstructionArgs struct {
	AccountIndex uint16
}

type SystemNonceInitInstructionAccounts struct {
	VmAuthority         ed25519.PublicKey
	Vm                  ed25519.PublicKey
	VmMemory            ed25519.PublicKey
	VirtualAccountOwner ed25519.PublicKey
}

func NewSystemNonceInitInstruction(
	accounts *SystemNonceInitInstructionAccounts,
	args *SystemNonceInitInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(SystemNonceInitInstructionDiscriminator)+
			SystemNonceInitInstructionArgsSize)

	putDiscriminator(data, SystemNonceInitInstructionDiscriminator, &offset)
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
