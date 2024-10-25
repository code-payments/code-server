package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var VmExecInstructionDiscriminator = []byte{
	0xe5, 0xcf, 0x51, 0x74, 0xed, 0x96, 0xba, 0x3e,
}

type VmExecArgsAndAccounts struct {
	Args     VmExecInstructionArgs
	Accounts VmExecInstructionAccounts
}

type VmExecInstructionArgs struct {
	Opcode     Opcode
	MemIndices []uint16
	MemBanks   []uint8
	Data       []uint8
}

type VmExecInstructionAccounts struct {
	VmAuthority     ed25519.PublicKey
	Vm              ed25519.PublicKey
	VmMemA          *ed25519.PublicKey
	VmMemB          *ed25519.PublicKey
	VmMemC          *ed25519.PublicKey
	VmMemD          *ed25519.PublicKey
	VmOmnibus       *ed25519.PublicKey
	VmRelay         *ed25519.PublicKey
	VmRelayVault    *ed25519.PublicKey
	ExternalAddress *ed25519.PublicKey
}

func NewVmExecInstruction(
	accounts *VmExecInstructionAccounts,
	args *VmExecInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(VmExecInstructionDiscriminator)+
			getVmExecInstructionArgSize(args))

	putDiscriminator(data, VmExecInstructionDiscriminator, &offset)
	putOpcode(data, args.Opcode, &offset)
	putUint16Array(data, args.MemIndices, &offset)
	putUint8Array(data, args.MemBanks, &offset)
	putUint8Array(data, args.Data, &offset)

	var tokenProgram *ed25519.PublicKey
	if accounts.VmOmnibus != nil || accounts.ExternalAddress != nil {
		tokenProgram = &SPL_TOKEN_PROGRAM_ID
	}

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
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmMemA),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmMemB),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmMemC),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmMemD),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmOmnibus),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmRelay),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.VmRelayVault),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(accounts.ExternalAddress),
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  getOptionalAccountMetaAddress(tokenProgram),
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}

func getVmExecInstructionArgSize(args *VmExecInstructionArgs) int {
	return (1 + // opcode
		4 + 2*len(args.MemIndices) + // mem_indices
		4 + len(args.MemBanks) + // mem_banks
		4 + len(args.Data)) // data
}
