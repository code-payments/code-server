package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var verifyProofInstructionDiscriminator = []byte{
	217, 211, 191, 110, 144, 13, 186, 98,
}

const (
	VerifyProofInstructionArgsSize = (1 + // pool_bump
		1) // proof_bump

	VerifyProofInstructionAccountsSize = (32 + // pool
		32 + // proof
		32 + // authority
		32) // payer

	VerifyProofInstructionSize = (8 + // discriminator
		VerifyProofInstructionArgsSize + // args
		VerifyProofInstructionAccountsSize) // accounts
)

type VerifyProofInstructionArgs struct {
	PoolBump  uint8
	ProofBump uint8
}

type VerifyProofInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Proof     ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewVerifyProofInstruction(
	accounts *VerifyProofInstructionAccounts,
	args *VerifyProofInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(verifyProofInstructionDiscriminator)+
			VerifyProofInstructionArgsSize)

	putDiscriminator(data, verifyProofInstructionDiscriminator, &offset)
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
		},
	}
}

func VerifyProofInstructionFromBinary(data []byte) (*VerifyProofInstructionArgs, *VerifyProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < VerifyProofInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, verifyProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args VerifyProofInstructionArgs
	var accounts VerifyProofInstructionAccounts

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
