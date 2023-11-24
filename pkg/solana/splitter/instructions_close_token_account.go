package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var closeTokenAccountInstructionDiscriminator = []byte{
	132, 172, 24, 60, 100, 156, 135, 97,
}

const (
	CloseTokenAccountInstructionArgsSize = (1 + // pool_bump
		1 + // proof_bump
		1) // vault_bump

	CloseTokenAccountInstructionAccountsSize = (32 + // pool
		32 + // proof
		32 + // commitmentVault
		32 + // poolVault
		32 + // authority
		32 + // payer
		32 + // tokenProgram
		32) // systemProgram

	CloseTokenAccountInstructionSize = (8 + // discriminator
		CloseTokenAccountInstructionArgsSize + // args
		CloseTokenAccountInstructionAccountsSize) // accounts
)

type CloseTokenAccountInstructionArgs struct {
	PoolBump  uint8
	ProofBump uint8
	VaultBump uint8
}

type CloseTokenAccountInstructionAccounts struct {
	Pool            ed25519.PublicKey
	Proof           ed25519.PublicKey
	CommitmentVault ed25519.PublicKey
	PoolVault       ed25519.PublicKey
	Authority       ed25519.PublicKey
	Payer           ed25519.PublicKey
}

func NewCloseTokenAccountInstruction(
	accounts *CloseTokenAccountInstructionAccounts,
	args *CloseTokenAccountInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(closeTokenAccountInstructionDiscriminator)+
			CloseTokenAccountInstructionArgsSize)

	putDiscriminator(data, closeTokenAccountInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)
	putUint8(data, args.ProofBump, &offset)
	putUint8(data, args.VaultBump, &offset)

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
				PublicKey:  accounts.CommitmentVault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.PoolVault,
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
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSTEM_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}

func CloseTokenAccountInstructionFromBinary(data []byte) (*CloseTokenAccountInstructionArgs, *CloseTokenAccountInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < CloseTokenAccountInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeTokenAccountInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseTokenAccountInstructionArgs
	var accounts CloseTokenAccountInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getUint8(data, &args.ProofBump, &offset)
	getUint8(data, &args.VaultBump, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Proof, &offset)
	getKey(data, &accounts.CommitmentVault, &offset)
	getKey(data, &accounts.PoolVault, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
