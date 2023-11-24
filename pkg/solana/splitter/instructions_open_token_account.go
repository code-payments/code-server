package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var openTokenAccountInstructionDiscriminator = []byte{
	77, 240, 240, 35, 150, 89, 234, 157,
}

const (
	OpenTokenAccountInstructionArgsSize = (1 + // PoolBump
		1) // ProofBump

	OpenTokenAccountInstructionAccountsSize = (32 + // pool
		32 + // proof
		32 + // commitmentVault
		32 + // mint
		32 + // authority
		32 + // payer
		32 + // splTokenProgram
		32 + // systemProgram
		32) // sysvarRent

	OpenTokenAccountInstructionSize = (8 + // discriminator
		OpenTokenAccountInstructionArgsSize + // args
		OpenTokenAccountInstructionAccountsSize) // accounts
)

type OpenTokenAccountInstructionArgs struct {
	PoolBump  uint8
	ProofBump uint8
}

type OpenTokenAccountInstructionAccounts struct {
	Pool            ed25519.PublicKey
	Proof           ed25519.PublicKey
	CommitmentVault ed25519.PublicKey
	Mint            ed25519.PublicKey
	Authority       ed25519.PublicKey
	Payer           ed25519.PublicKey
}

func NewOpenTokenAccountInstruction(
	accounts *OpenTokenAccountInstructionAccounts,
	args *OpenTokenAccountInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(openTokenAccountInstructionDiscriminator)+
			OpenTokenAccountInstructionArgsSize)

	putDiscriminator(data, openTokenAccountInstructionDiscriminator, &offset)
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
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Proof,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.CommitmentVault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
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

func OpenTokenAccountInstructionFromBinary(data []byte) (*OpenTokenAccountInstructionArgs, *OpenTokenAccountInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < OpenTokenAccountInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, openTokenAccountInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args OpenTokenAccountInstructionArgs
	var accounts OpenTokenAccountInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getUint8(data, &args.ProofBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Proof, &offset)
	getKey(data, &accounts.CommitmentVault, &offset)
	getKey(data, &accounts.Mint, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
