package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var closeProofInstructionDiscriminator = []byte{
	64, 76, 168, 8, 126, 109, 164, 179,
}

const (
	CloseProofInstructionArgsSize = (1 + // pool_bump
		1) // proof_bump

	CloseProofInstructionAccountsSize = (32 + // pool
		32 + // proof
		32 + // authority
		32 + // payer
		32 + // systemProgram
		32) // rentSysvar

	CloseProofInstructionSize = (8 + // discriminator
		CloseProofInstructionArgsSize + // args
		CloseProofInstructionAccountsSize) // accounts
)

type CloseProofInstructionArgs struct {
	PoolBump  uint8
	ProofBump uint8
}

type CloseProofInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Proof     ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewCloseProofInstruction(
	accounts *CloseProofInstructionAccounts,
	args *CloseProofInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(closeProofInstructionDiscriminator)+
			CloseProofInstructionArgsSize)

	putDiscriminator(data, closeProofInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)
	putUint8(data, args.ProofBump, &offset)

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
				PublicKey:  accounts.Proof,
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

func CloseProofInstructionFromBinary(data []byte) (*CloseProofInstructionArgs, *CloseProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < CloseProofInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseProofInstructionArgs
	var accounts CloseProofInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getUint8(data, &args.ProofBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Proof, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
