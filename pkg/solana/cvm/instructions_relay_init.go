package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var RelayInitInstructionDiscriminator = []byte{
	0x72, 0x6a, 0x94, 0xd1, 0x3c, 0x83, 0xb4, 0xe1,
}

const (
	RelayInitInstructionArgsSize = (1 + // num_levels
		1 + // num_history
		MaxRelayAccountNameSize) // name
)

type RelayInitInstructionArgs struct {
	NumLevels  uint8
	NumHistory uint8
	Name       string
}

type RelayInitInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	Relay       ed25519.PublicKey
	RelayVault  ed25519.PublicKey
	Mint        ed25519.PublicKey
}

func NewRelayInitInstruction(
	accounts *RelayInitInstructionAccounts,
	args *RelayInitInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(RelayInitInstructionDiscriminator)+
			RelayInitInstructionArgsSize)

	putDiscriminator(data, RelayInitInstructionDiscriminator, &offset)
	putUint8(data, args.NumLevels, &offset)
	putUint8(data, args.NumHistory, &offset)
	putString(data, args.Name, &offset)

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
				PublicKey:  accounts.Relay,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.RelayVault,
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
