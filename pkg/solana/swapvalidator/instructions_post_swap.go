package swap_validator

import (
	"crypto/ed25519"
)

var postSwapInstructionDiscriminator = []byte{
	0x9f, 0xd5, 0xb7, 0x39, 0xb3, 0x8a, 0x75, 0xa1,
}

const (
	PostSwapInstructionArgsSize = (1 + // StateBump
		8 + // MaxToSend
		8) // MinToReceive
)

type PostSwapInstructionArgs struct {
	StateBump    uint8
	MaxToSend    uint64
	MinToReceive uint64
}

type PostSwapInstructionAccounts struct {
	PreSwapState ed25519.PublicKey
	Source       ed25519.PublicKey
	Destination  ed25519.PublicKey
	Payer        ed25519.PublicKey
}

func NewPostSwapInstruction(
	accounts *PostSwapInstructionAccounts,
	args *PostSwapInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(postSwapInstructionDiscriminator)+
			PostSwapInstructionArgsSize)

	putDiscriminator(data, postSwapInstructionDiscriminator, &offset)
	putUint8(data, args.StateBump, &offset)
	putUint64(data, args.MaxToSend, &offset)
	putUint64(data, args.MinToReceive, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.PreSwapState,
				IsWritable: true,
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
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
			},
		},
	}
}
