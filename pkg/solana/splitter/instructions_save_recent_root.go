package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var saveRecentRootInstructionDiscriminator = []byte{
	163, 45, 123, 32, 127, 86, 42, 189,
}

const (
	SaveRecentRootInstructionArgsSize = 1 // pool_bump

	SaveRecentRootInstructionAccountsSize = (32 + // pool
		32 + // authority
		32) // payer

	SaveRecentRootInstructionSize = (8 + // discriminator
		SaveRecentRootInstructionArgsSize + // args
		SaveRecentRootInstructionAccountsSize) // accounts
)

type SaveRecentRootInstructionArgs struct {
	PoolBump uint8
}

type SaveRecentRootInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewSaveRecentRootInstruction(
	accounts *SaveRecentRootInstructionAccounts,
	args *SaveRecentRootInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(saveRecentRootInstructionDiscriminator)+
			SaveRecentRootInstructionArgsSize)

	putDiscriminator(data, saveRecentRootInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Pool,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Authority,
				IsWritable: false,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
			},
		},
	}
}

func SaveRecentRootInstructionFromBinary(data []byte) (*SaveRecentRootInstructionArgs, *SaveRecentRootInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < SaveRecentRootInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, saveRecentRootInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args SaveRecentRootInstructionArgs
	var accounts SaveRecentRootInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
