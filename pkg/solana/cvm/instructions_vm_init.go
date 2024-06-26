package cvm

import (
	"crypto/ed25519"
)

var VmInitInstructionDiscriminator = []byte{
	0xd3, 0xd4, 0x60, 0x15, 0xc2, 0x62, 0xe1, 0xfc,
}

const (
	VmInitInstructionArgsSize = 1 // lock_duration
)

type VmInitInstructionArgs struct {
	LockDuration uint8
}

type VmInitInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmOmnibus   ed25519.PublicKey
	Mint        ed25519.PublicKey
}

func NewVmInitInstruction(
	accounts *VmInitInstructionAccounts,
	args *VmInitInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(VmInitInstructionDiscriminator)+
			VmInitInstructionArgsSize)

	putDiscriminator(data, VmInitInstructionDiscriminator, &offset)
	putUint8(data, args.LockDuration, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
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
				PublicKey:  accounts.VmOmnibus,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
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
