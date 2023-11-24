package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var initializeProofInstructionDiscriminator = []byte{
	165, 188, 188, 88, 25, 248, 86, 93,
}

const (
	InitializeProofInstructionArgsSize = (1 + // PoolBump
		32 + // MerkleRoot
		32) // Commitment

	InitializeProofInstructionAccountsSize = (32 + // pool
		32 + // proof
		32 + // authority
		32 + // payer
		32 + // systemProgram
		32) // sysvarRent

	InitializeProofInstructionSize = (8 + // discriminator
		InitializeProofInstructionArgsSize + // args
		InitializeProofInstructionAccountsSize) // accounts
)

type InitializeProofInstructionArgs struct {
	PoolBump   uint8
	MerkleRoot Hash
	Commitment ed25519.PublicKey
}

type InitializeProofInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Proof     ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewInitializeProofInstruction(
	accounts *InitializeProofInstructionAccounts,
	args *InitializeProofInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(initializeProofInstructionDiscriminator)+
			InitializeProofInstructionArgsSize)

	putDiscriminator(data, initializeProofInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)
	putHash(data, args.MerkleRoot, &offset)
	putKey(data, args.Commitment, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Pool,
				IsWritable: false,
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

func InitializeProofInstructionFromBinary(data []byte) (*InitializeProofInstructionArgs, *InitializeProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < InitializeProofInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, initializeProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args InitializeProofInstructionArgs
	var accounts InitializeProofInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getHash(data, &args.MerkleRoot, &offset)
	getKey(data, &args.Commitment, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Proof, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
