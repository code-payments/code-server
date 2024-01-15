package swap_validator

import (
	"crypto/ed25519"
)

var preSwapInstructionDiscriminator = []byte{
	0xb7, 0xdd, 0xc7, 0x8a, 0xcf, 0x49, 0x7f, 0x71,
}

const (
	PreSwapInstructionArgsSize = 0
)

type PreSwapInstructionArgs struct {
}

type PreSwapInstructionAccounts struct {
	PreSwapState      ed25519.PublicKey
	User              ed25519.PublicKey
	Source            ed25519.PublicKey
	Destination       ed25519.PublicKey
	Nonce             ed25519.PublicKey
	Payer             ed25519.PublicKey
	RemainingAccounts []AccountMeta
}

func NewPreSwapInstruction(
	accounts *PreSwapInstructionAccounts,
	args *PreSwapInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(preSwapInstructionDiscriminator)+
			PreSwapInstructionArgsSize)

	putDiscriminator(data, preSwapInstructionDiscriminator, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: append(
			[]AccountMeta{
				{
					PublicKey:  accounts.PreSwapState,
					IsWritable: true,
					IsSigner:   false,
				},
				{
					PublicKey:  accounts.User,
					IsWritable: false,
					IsSigner:   false,
				},
				{
					PublicKey:  accounts.Source,
					IsWritable: false,
					IsSigner:   false,
				},
				{
					PublicKey:  accounts.Destination,
					IsWritable: false,
					IsSigner:   false,
				},
				{
					PublicKey:  accounts.Nonce,
					IsWritable: false,
					IsSigner:   false,
				},
				{
					PublicKey:  accounts.Payer,
					IsWritable: true,
					IsSigner:   true,
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
			accounts.RemainingAccounts...,
		),
	}
}
